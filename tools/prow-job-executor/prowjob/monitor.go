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

package prowjob

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/go-logr/logr"

	prowjobs "sigs.k8s.io/prow/pkg/apis/prowjobs/v1"
	prowgangway "sigs.k8s.io/prow/pkg/gangway"
)

// JobFailedError indicates a Prow job completed in the "failure" state - as
// opposed to "error" or "aborted", which usually indicate an infra problem or
// a cancellation rather than a test failure. Timeouts are reported separately
// as plain errors, not as JobFailedError. Only jobs that failed this way are
// candidates for the EV2 auto-retry, since that's the class of failure the
// allow-retry test label and finished.json retry metadata are about (see
// AROSLSRE-1721).
type JobFailedError struct {
	ProwExecutionID string
	JobURL          string
}

func (e *JobFailedError) Error() string {
	return fmt.Sprintf("job %s failed - check the Prow UI for detailed logs: %s", e.ProwExecutionID, e.JobURL)
}

// EV2RetryableMarker is the fixed, matchable substring prow-job-executor emits when a
// gating job failure is eligible for an automatic EV2 retry (see ev2RetryEligible). The
// EV2 gating step's pipeline.yaml configures automatedRetry.errorContainsAny with this
// marker, so EV2 - not prow-job-executor - re-runs the whole step from scratch when it
// sees the marker in the step's captured error output. prow-job-executor never resubmits
// the job itself; it only decides whether the failure qualifies and surfaces that
// decision through the error text.
const EV2RetryableMarker = "ev2-retryable-known-issue-failure"

// EV2RetryableError wraps a job failure that finished.json's metadata marks as safe to
// auto-retry. Its Error() text always contains EV2RetryableMarker, so EV2's
// automatedRetry.errorContainsAny can match on it and re-run the gating step.
type EV2RetryableError struct {
	Cause error
}

func (e *EV2RetryableError) Error() string {
	return fmt.Sprintf("%s: %s", EV2RetryableMarker, e.Cause.Error())
}

func (e *EV2RetryableError) Unwrap() error {
	return e.Cause
}

// Monitor handles job execution and monitoring
type Monitor struct {
	client               *Client
	pollInterval         time.Duration
	timeout              time.Duration
	dryRun               bool
	gatePromotion        bool
	allowEV2Retry        bool
	maxAutoRetryFailures int

	// checkRetryMarker fetches finished.json for a job's status URL and reports
	// whether its metadata marks the run as safe to auto-retry. Defaults to
	// jobAllowsEV2Retry; overridable in tests.
	checkRetryMarker func(ctx context.Context, jobURL string, maxAutoRetryFailures int) (bool, error)
}

// NewMonitor creates a new job monitor with the specified polling interval and timeout.
// allowEV2Retry opts into checking a failed job's finished.json metadata and, if it marks
// the failure as safe to retry (see AROSLSRE-1721), failing with EV2RetryableError instead
// of a plain JobFailedError - so the EV2 gating step's automatedRetry re-runs the whole
// step, rather than prow-job-executor resubmitting the job itself. It has no effect unless
// gatePromotion is also true. maxAutoRetryFailures caps how many failed tests a run may
// have (all labeled allow-retry) and still qualify; pass DefaultMaxEV2AutoRetryFailures
// unless a caller wants to tune it without an ARO-HCP rebuild.
func NewMonitor(client *Client, pollInterval, timeout time.Duration, dryRun, gatePromotion, allowEV2Retry bool, maxAutoRetryFailures int) *Monitor {
	return &Monitor{
		client:               client,
		pollInterval:         pollInterval,
		timeout:              timeout,
		dryRun:               dryRun,
		gatePromotion:        gatePromotion,
		allowEV2Retry:        allowEV2Retry,
		maxAutoRetryFailures: maxAutoRetryFailures,
		checkRetryMarker:     jobAllowsEV2Retry,
	}
}

// WaitForCompletion polls job status until completion
func (m *Monitor) WaitForCompletion(ctx context.Context, logger logr.Logger, prowExecutionID string) error {
	ctx, cancel := context.WithTimeout(ctx, m.timeout)
	defer cancel()

	// Create ticker for polling interval
	ticker := time.NewTicker(m.pollInterval)
	defer ticker.Stop()

	// Check status immediately, then poll at intervals
	for {
		job, err := m.client.GetJobStatus(ctx, prowExecutionID)
		if err != nil {
			logger.Error(err, "Failed to get job status after retries, will continue polling")
		} else {
			status := string(job.Status.State)
			logger = logger.WithValues(
				"prowExecutionID", prowExecutionID,
				"status", status,
				"jobName", job.Spec.Job,
				"prowUrl", job.Status.URL,
			)
			logger.Info("Job status update")

			switch status {
			case string(prowjobs.SuccessState):
				logger.Info("Job completed successfully")
				return nil
			case string(prowjobs.FailureState):
				if m.gatePromotion {
					return &JobFailedError{ProwExecutionID: prowExecutionID, JobURL: job.Status.URL}
				} else {
					logger.Info("Unexpected job state, but gating is not requested.")
					return nil
				}
			case string(prowjobs.ErrorState):
				if m.gatePromotion {
					return fmt.Errorf("job %s encountered an error - check Prow status page and job logs for details: %s", prowExecutionID, job.Status.URL)
				} else {
					logger.Error(err, "Unexpected job state, but gating is not requested.")
					return nil
				}
			case string(prowjobs.AbortedState):
				if m.gatePromotion {
					return fmt.Errorf("job %s was aborted - this may be due to timeout or manual cancellation", prowExecutionID)
				} else {
					logger.Error(err, "Unexpected job state, but gating is not requested.")
					return nil
				}
			}
		}

		select {
		case <-ctx.Done():
			if job != nil {
				return fmt.Errorf("job monitoring timed out after %v - job %s may still be running, check Prow UI: %s", m.timeout, prowExecutionID, job.Status.URL)
			}
			return fmt.Errorf("job monitoring timed out after %v - job %s may still be running (unable to retrieve job status)", m.timeout, prowExecutionID)
		case <-ticker.C:
			// Continue to next iteration
		}
	}
}

// ExecuteAndWait submits a job once and waits for completion. It never resubmits the job
// itself. If the job fails (JobFailedError) and this Monitor has allowEV2Retry set, it
// fetches the failed job's finished.json and, if its metadata marks it as safe to retry
// (only known-issue tests failed, see AROSLSRE-1721), returns an EV2RetryableError instead
// of the plain JobFailedError. The EV2 gating step's pipeline.yaml matches
// EV2RetryableMarker via automatedRetry.errorContainsAny and re-runs the whole step from
// scratch - prow-job-executor only decides eligibility, EV2 owns the actual retry.
func (m *Monitor) ExecuteAndWait(ctx context.Context, logger logr.Logger, request *prowgangway.CreateJobExecutionRequest) error {
	ctx, cancel := context.WithTimeout(ctx, m.timeout)
	defer cancel()

	// Submit job
	logger.Info("Submitting Prow job", "jobName", request.JobName)
	if m.dryRun {
		logger.Info("Dry-run is set, exiting.")
		return nil
	}
	prowExecutionID, err := m.client.SubmitJob(ctx, request)
	if err != nil {
		return fmt.Errorf("failed to submit job: %w", err)
	}

	logger.Info("Job submitted successfully", "prowExecutionID", prowExecutionID, "jobName", request.JobName)

	err = m.WaitForCompletion(ctx, logger, prowExecutionID)
	if err == nil || !m.allowEV2Retry {
		return err
	}

	var failedErr *JobFailedError
	if !errors.As(err, &failedErr) {
		return err
	}

	retry, checkErr := m.checkRetryMarker(ctx, failedErr.JobURL, m.maxAutoRetryFailures)
	if checkErr != nil {
		logger.Error(checkErr, "Failed to inspect finished.json for the EV2 retry signal, failing normally", "prowExecutionID", failedErr.ProwExecutionID)
		return err
	}
	if !retry {
		return err
	}

	logger.Info("finished.json marks the run as safe to retry, failing with the EV2-retryable marker so the gating step's automatedRetry re-runs it", "prowExecutionID", failedErr.ProwExecutionID, "jobURL", failedErr.JobURL)
	return &EV2RetryableError{Cause: err}
}
