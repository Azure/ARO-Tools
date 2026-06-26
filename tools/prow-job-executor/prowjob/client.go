// Copyright 2025 Microsoft Corporation
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package prowjob provides functionality for interacting with OpenShift Prow jobs,
// including job submission, status monitoring, and authentication via Azure Key Vault.
package prowjob

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"time"

	"github.com/go-logr/logr"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/known/timestamppb"

	"k8s.io/apimachinery/pkg/util/wait"

	prowjobs "sigs.k8s.io/prow/pkg/apis/prowjobs/v1"
	prowgangway "sigs.k8s.io/prow/pkg/gangway"
	"sigs.k8s.io/yaml"

	"github.com/Azure/ARO-Tools/tools/prow-job-executor/internal/retry"
)

// bulkStatusChangePath is the Gangway REST route for BulkJobStatusChange; it
// shares the host of the executions endpoint.
const bulkStatusChangePath = "/v1/bulk-job-status-update"

// jobSubmissionResponse represents the minimal JSON response from Gangway API for job submission
type jobSubmissionResponse struct {
	ID string `json:"id"`
}

// Client handles Prow API interactions
type Client struct {
	token         string
	client        *http.Client
	gangwayURL    string
	bulkURL       string
	prowURL       string
	submitBackoff wait.Backoff
}

// NewClient creates a new Prow API client with the provided authentication token and API URLs.
func NewClient(token, gangwayURL, prowURL string) *Client {
	return &Client{
		token:      token,
		gangwayURL: gangwayURL,
		bulkURL:    deriveBulkURL(gangwayURL),
		prowURL:    prowURL,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
		// Exponential backoff with jitter for transient job-submission failures.
		// Delays between attempts grow 1, 2, 4, 8, 16, 32 minutes over up to 7
		// attempts, for a worst-case cumulative wait of ~63 minutes before giving
		// up. The parent context still bounds the total runtime.
		//
		// Note: apimachinery's Backoff.Cap not only clamps an individual delay but
		// also stops all further retries once the cap is reached, so the schedule
		// is deliberately bounded by Steps rather than Cap.
		submitBackoff: wait.Backoff{
			Duration: time.Minute, // Initial delay
			Factor:   2.0,         // Exponential factor
			Jitter:   0.1,         // 10% jitter to de-sync concurrent submitters
			Steps:    7,           // Maximum attempts (~63m worst-case cumulative wait)
		},
	}
}

// deriveBulkURL returns the Gangway bulk job status-change endpoint that shares
// the host of the executions endpoint. If gangwayURL cannot be parsed the input
// is returned unchanged so the caller still surfaces a clear HTTP error instead
// of panicking.
func deriveBulkURL(gangwayURL string) string {
	u, err := url.Parse(gangwayURL)
	if err != nil {
		return gangwayURL
	}
	u.Path = bulkStatusChangePath
	u.RawQuery = ""
	return u.String()
}

// transient errors with exponential backoff.
//
// The Gangway API enforces a low request-rate limit (~9 requests/minute per client
// IP) and rejects excess requests immediately with HTTP 429 rather than queueing
// them. Without retries a momentary rate-limit fails the whole EV2 gating step,
// forcing the entire deployment job to be restarted. Only transient failures
// (HTTP 429, 5xx, and network errors) are retried; everything else fails fast.
func (c *Client) SubmitJob(ctx context.Context, request *prowgangway.CreateJobExecutionRequest) (string, error) {
	return retry.WithValue(ctx, c.submitBackoff, isRetryableError, func(ctx context.Context) (string, error) {
		return c.submitJobOnce(ctx, request)
	})
}

// submitJobOnce performs a single job submission request without retry logic.
func (c *Client) submitJobOnce(ctx context.Context, request *prowgangway.CreateJobExecutionRequest) (string, error) {
	logger, err := logr.FromContext(ctx)
	if err != nil {
		return "", err
	}

	data, err := json.Marshal(request)
	if err != nil {
		return "", fmt.Errorf("failed to marshal job request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.gangwayURL, bytes.NewBuffer(data))
	if err != nil {
		return "", fmt.Errorf("failed to create HTTP request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to submit job: %w", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			logger.Error(err, "failed to close body")
		}
	}()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		err := fmt.Errorf("job submission failed with status %d: %s", resp.StatusCode, string(body))
		return "", &httpStatusError{statusCode: resp.StatusCode, err: err}
	}

	var response jobSubmissionResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return "", fmt.Errorf("failed to decode job response: %w", err)
	}

	return response.ID, nil
}

// GetJobStatus retrieves the full job information by Prow execution ID with retry logic
func (c *Client) GetJobStatus(ctx context.Context, prowExecutionID string) (*prowjobs.ProwJob, error) {
	// Configure exponential backoff with jitter
	backoff := wait.Backoff{
		Duration: time.Second,      // Initial delay
		Factor:   2.0,              // Exponential factor
		Jitter:   0.1,              // 10% jitter
		Steps:    3,                // Maximum attempts
		Cap:      10 * time.Second, // Maximum delay cap
	}

	// Everything except a non-retryable HTTP status (e.g. 401/403) is retried: a
	// freshly submitted job's status may 404 until it propagates, and transport
	// errors are transient.
	isRetryable := func(err error) bool {
		return !isNonRetryableHTTPError(err)
	}

	return retry.WithValue(ctx, backoff, isRetryable, func(ctx context.Context) (*prowjobs.ProwJob, error) {
		return c.getJobStatusOnce(ctx, prowExecutionID)
	})
}

// getJobStatusOnce performs a single job status request without retry logic
func (c *Client) getJobStatusOnce(ctx context.Context, prowExecutionID string) (*prowjobs.ProwJob, error) {
	logger, err := logr.FromContext(ctx)
	if err != nil {
		return nil, err
	}

	url := fmt.Sprintf("%s?prowjob=%s", c.prowURL, prowExecutionID)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP request: %w", err)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to get job status: %w", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			logger.Error(err, "failed to close body")
		}
	}()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		err := fmt.Errorf("failed to get job status, status code: %d, body: %s", resp.StatusCode, string(body))
		return nil, &httpStatusError{statusCode: resp.StatusCode, err: err}
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	var prowJob prowjobs.ProwJob
	if err := yaml.Unmarshal(body, &prowJob); err != nil {
		bodyStr := fmt.Sprintf("%.1000s", string(body))
		return nil, fmt.Errorf("failed to decode job status as YAML: %w, response body: %s", err, bodyStr)
	}

	return &prowJob, nil
}

// ev2RolloutRegionAnnotation is the ProwJob annotation carrying the EV2 rollout
// region. It mirrors the key set by the executor when submitting the job and is
// used here only for logging, since the Gangway abort API cannot filter on it.
const ev2RolloutRegionAnnotation = "ev2.rollout/region"

// AbortJob requests cancellation of a running Prow job identified by its
// execution ID.
//
// The Gangway API exposes no per-execution abort. The only available mechanism
// is BulkJobStatusChange, which selects jobs by type, refs (org/repo), state and
// a StartTime window; it cannot target a single execution ID, and crucially it
// cannot filter by region. The same EV2 pipeline fans a rollout out to several
// regions concurrently, and every regional execution of an environment shares
// the same job name and refs - the region lives only in an annotation/env var
// that the selector ignores. ProwJob StartTime is also serialized at one-second
// precision, so a [StartTime, StartTime] window matches every execution that
// started in that same second.
//
// To avoid cancelling another region's job, AbortJob first enumerates the
// concurrent executions of the same job in the same state and verifies that our
// target is the only one whose start time falls in the bulk window. If a sibling
// (e.g. another region of the same rollout) shares that window we cannot isolate
// our execution, so the abort is skipped rather than risk cancelling the wrong
// region's E2E run.
//
// Aborting is best-effort and idempotent: a terminal job is a no-op, a job with
// no recorded StartTime is skipped, and an un-isolable job is skipped.
func (c *Client) AbortJob(ctx context.Context, prowExecutionID string) error {
	logger, err := logr.FromContext(ctx)
	if err != nil {
		return err
	}

	job, err := c.GetJobStatus(ctx, prowExecutionID)
	if err != nil {
		return fmt.Errorf("failed to look up job %s before aborting: %w", prowExecutionID, err)
	}

	state := job.Status.State
	region := job.Annotations[ev2RolloutRegionAnnotation]
	logger = logger.WithValues("prowExecutionID", prowExecutionID, "jobName", job.Spec.Job, "status", string(state), "region", region)

	if isTerminalState(state) {
		logger.Info("Job already in a terminal state, nothing to abort")
		return nil
	}

	if job.Status.StartTime.IsZero() {
		// Without a recorded StartTime the bulk selector cannot be bounded at all,
		// so we refuse to issue an abort that could cancel sibling jobs sharing the
		// same type and refs.
		logger.Info("Job has no recorded start time yet; skipping abort to avoid affecting other jobs")
		return nil
	}

	// Region-aware isolation check: ensure no other concurrent execution of the
	// same job in the same state shares the StartTime window we are about to
	// abort. Because the bulk API is region-blind and second-precise, aborting
	// when a sibling shares the window would cancel that sibling too.
	isolated, err := c.abortWindowIsIsolated(ctx, logger, job, prowExecutionID)
	if err != nil {
		// If we cannot prove isolation we err on the side of caution and leave the
		// job running rather than risk cancelling another region's execution.
		logger.Error(err, "Could not verify the abort would be isolated to this execution; skipping abort")
		return nil
	}
	if !isolated {
		return nil
	}

	var refs *prowgangway.Refs
	if job.Spec.Refs != nil {
		refs, err = prowgangway.FromCrdRefs(job.Spec.Refs)
		if err != nil {
			return fmt.Errorf("failed to convert refs for job %s: %w", prowExecutionID, err)
		}
	}

	// Pin the window to the StartTime; isMatchingCondition treats the bounds
	// inclusively, so [StartTime, StartTime] is the tightest window the API allows.
	start := timestamppb.New(job.Status.StartTime.Time)
	request := &prowgangway.BulkJobStatusChangeRequest{
		JobStatusChange: &prowgangway.JobStatusChange{
			Current: prowgangway.TranslateProwJobStatus(&job.Status),
			Desired: prowgangway.JobExecutionStatus_ABORTED,
		},
		JobType:       prowgangway.TranslateProwJobType(job.Spec.Type),
		Refs:          refs,
		StartedAfter:  start,
		StartedBefore: start,
	}

	logger.Info("Requesting abort of Prow job")
	if err := c.postBulkStatusChange(ctx, request); err != nil {
		return fmt.Errorf("failed to abort job %s: %w", prowExecutionID, err)
	}
	logger.Info("Abort request sent for Prow job")
	return nil
}

// abortWindowIsIsolated reports whether the target job is the only concurrent
// execution of the same job (across all regions) whose StartTime falls within
// the one-second bulk-abort window. It returns false (with a clear log) when a
// sibling shares the window, so the caller can skip the abort instead of
// cancelling another region's job.
func (c *Client) abortWindowIsIsolated(ctx context.Context, logger logr.Logger, job *prowjobs.ProwJob, prowExecutionID string) (bool, error) {
	targetSecond := job.Status.StartTime.Truncate(time.Second)
	currentStatus := prowgangway.TranslateProwJobStatus(&job.Status)

	executions, err := c.ListExecutions(ctx, job.Spec.Job, currentStatus)
	if err != nil {
		return false, fmt.Errorf("failed to list concurrent executions of job %q: %w", job.Spec.Job, err)
	}

	for _, exec := range executions {
		siblingID := exec.GetId()
		if siblingID == "" || siblingID == prowExecutionID {
			continue
		}

		sibling, err := c.GetJobStatus(ctx, siblingID)
		if err != nil {
			// We could not inspect a concurrent execution, so we cannot rule out a
			// shared window; treat that as un-isolable.
			return false, fmt.Errorf("failed to inspect concurrent execution %s: %w", siblingID, err)
		}
		if sibling.Status.StartTime.IsZero() {
			// Not started yet, so it cannot be in our window.
			continue
		}
		if sibling.Status.StartTime.Truncate(time.Second).Equal(targetSecond) {
			logger.Info("Skipping abort: a concurrent execution shares the abort window and cannot be isolated; leaving the Prow job running",
				"conflictingExecutionID", siblingID,
				"conflictingRegion", sibling.Annotations[ev2RolloutRegionAnnotation],
				"startWindow", targetSecond.UTC().Format(time.RFC3339))
			return false, nil
		}
	}

	return true, nil
}

// ListExecutions returns the executions of the given job name filtered to the
// provided status, via the Gangway ListJobExecutions endpoint.
func (c *Client) ListExecutions(ctx context.Context, jobName string, status prowgangway.JobExecutionStatus) ([]*prowgangway.JobExecution, error) {
	logger, err := logr.FromContext(ctx)
	if err != nil {
		return nil, err
	}

	query := url.Values{}
	query.Set("job_name", jobName)
	if status != prowgangway.JobExecutionStatus_JOB_EXECUTION_STATUS_UNSPECIFIED {
		query.Set("status", status.String())
	}
	requestURL := c.gangwayURL + "?" + query.Encode()

	req, err := http.NewRequestWithContext(ctx, "GET", requestURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.token)

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to list executions: %w", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			logger.Error(err, "failed to close body")
		}
	}()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, &httpStatusError{statusCode: resp.StatusCode, err: fmt.Errorf("list executions failed with status %d: %s", resp.StatusCode, string(body))}
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	var executions prowgangway.JobExecutions
	if err := protojson.Unmarshal(body, &executions); err != nil {
		return nil, fmt.Errorf("failed to decode executions list: %w", err)
	}
	return executions.GetJobExecution(), nil
}

// isTerminalState reports whether a ProwJob state is final and therefore not
// abortable.
func isTerminalState(state prowjobs.ProwJobState) bool {
	switch state {
	case prowjobs.SuccessState, prowjobs.FailureState, prowjobs.AbortedState, prowjobs.ErrorState:
		return true
	default:
		return false
	}
}

// postBulkStatusChange POSTs a BulkJobStatusChangeRequest to Gangway, retrying on
// transient errors with a short exponential backoff.
func (c *Client) postBulkStatusChange(ctx context.Context, request *prowgangway.BulkJobStatusChangeRequest) error {
	logger, err := logr.FromContext(ctx)
	if err != nil {
		return err
	}

	// protojson is required here: the Gangway REST gateway expects proto3 JSON,
	// where enums are strings and Timestamps are RFC 3339 strings. Plain
	// encoding/json would emit integer enums and {seconds,nanos} timestamps that
	// the gateway rejects.
	data, err := protojson.Marshal(request)
	if err != nil {
		return fmt.Errorf("failed to marshal bulk status change request: %w", err)
	}

	backoff := wait.Backoff{
		Duration: time.Second,
		Factor:   2.0,
		Jitter:   0.1,
		Steps:    3,
		Cap:      10 * time.Second,
	}

	var lastErr error
	condition := func(ctx context.Context) (bool, error) {
		if err := c.postBulkStatusChangeOnce(ctx, data); err != nil {
			lastErr = err
			if !isRetryableError(err) {
				return false, err
			}
			logger.Info("Bulk status change failed with a transient error, will retry", "error", err.Error())
			return false, nil
		}
		return true, nil
	}

	if err := wait.ExponentialBackoffWithContext(ctx, backoff, condition); err != nil {
		if lastErr != nil {
			return fmt.Errorf("bulk status change failed after retries: %w", lastErr)
		}
		return err
	}
	return nil
}

// postBulkStatusChangeOnce performs a single bulk status-change request without
// retry logic.
func (c *Client) postBulkStatusChangeOnce(ctx context.Context, data []byte) error {
	logger, err := logr.FromContext(ctx)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.bulkURL, bytes.NewBuffer(data))
	if err != nil {
		return fmt.Errorf("failed to create HTTP request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send bulk status change: %w", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			logger.Error(err, "failed to close body")
		}
	}()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		err := fmt.Errorf("bulk status change failed with status %d: %s", resp.StatusCode, string(body))
		return &httpStatusError{statusCode: resp.StatusCode, err: err}
	}
	return nil
}

// httpStatusError wraps errors with HTTP status code information
type httpStatusError struct {
	statusCode int
	err        error
}

func (e *httpStatusError) Error() string {
	return e.err.Error()
}

func (e *httpStatusError) Unwrap() error {
	return e.err
}

// isNonRetryableHTTPError checks if an error represents a non-retryable HTTP status
func isNonRetryableHTTPError(err error) bool {
	var httpErr *httpStatusError
	if errors.As(err, &httpErr) {
		return !isRetryableStatusCode(httpErr.statusCode)
	}
	return false
}

// isRetryableError reports whether a job-submission error is transient and worth
// retrying. Only HTTP 429, 5xx responses and network-level errors are retried;
// deterministic failures (request marshaling/construction, response decoding, and
// other 4xx such as 400/404/409) are treated as permanent and fail fast.
func isRetryableError(err error) bool {
	var httpErr *httpStatusError
	if errors.As(err, &httpErr) {
		code := httpErr.statusCode
		return code == http.StatusTooManyRequests || (code >= 500 && code <= 599)
	}

	// Transport/network errors (timeouts, connection resets, etc.) are transient.
	var netErr net.Error
	return errors.As(err, &netErr)
}

// isRetryableStatusCode determines if an HTTP status code should be retried
func isRetryableStatusCode(statusCode int) bool {
	switch statusCode {
	case http.StatusUnauthorized, // 401
		http.StatusForbidden: // 403
		return false
	default:
		return true
	}
}
