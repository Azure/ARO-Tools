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

// abortTimeout bounds the best-effort abort issued when monitoring is cancelled.
// It must stay well within the process's shutdown grace period so the request can
// be sent before the container is killed.
const abortTimeout = 30 * time.Second

// Monitor handles job execution and monitoring
type Monitor struct {
	client               *Client
	pollInterval         time.Duration
	timeout              time.Duration
	dryRun               bool
	gatePromotion        bool
	allowEV2Retry        bool
	abortOnCancel        bool
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
func NewMonitor(client *Client, pollInterval, timeout time.Duration, dryRun, gatePromotion, allowEV2Retry, abortOnCancel bool, maxAutoRetryFailures int) *Monitor {
	return &Monitor{
		client:               client,
		pollInterval:         pollInterval,
		timeout:              timeout,
		dryRun:               dryRun,
		gatePromotion:        gatePromotion,
		allowEV2Retry:        allowEV2Retry,
		abortOnCancel:        abortOnCancel,
		maxAutoRetryFailures: maxAutoRetryFailures,
		checkRetryMarker:     jobAllowsEV2Retry,
	}
}

// JobOutcome carries the result of waiting for a Prow job, with retry eligibility as an
// explicit field rather than something callers have to infer by type-asserting Err. Only a
// job that completed in the "failure" state (as opposed to "error"/"aborted", which usually
// indicate an infra problem or a cancellation) is ever Retryable; ExecuteAndWait uses this
// field directly instead of errors.As-ing for *JobFailedError.
type JobOutcome struct {
	// Err is nil on success, otherwise the reason WaitForCompletion stopped waiting.
	Err error
	// Retryable is true only when Err is non-nil and came from the job finishing in the
	// "failure" state - the class of failure the EV2 auto-retry (AROSLSRE-1721) applies to.
	Retryable bool
	// JobURL is the Prow status page URL. It's only populated when Err is non-nil and
	// gating was requested (FailureState/ErrorState/AbortedState with m.gatePromotion set,
	// or a timeout after a status was observed) - it's empty on success and on the
	// non-gating "unexpected status" paths, since there's no failure to report there.
	JobURL string
}

// WaitForCompletion polls job status until completion. Cancellation of ctx is
// treated as a signal to abort the Prow job when abortOnCancel is set.
func (m *Monitor) WaitForCompletion(ctx context.Context, logger logr.Logger, prowExecutionID string) error {
	return m.waitForCompletion(ctx, logger, prowExecutionID).Err
}

// waitForCompletion is the shared polling implementation. It returns a JobOutcome so callers
// that need to distinguish a retry-eligible job failure (ExecuteAndWait) can check
// outcome.Retryable directly, without control flow via typed-error inspection.
func (m *Monitor) waitForCompletion(ctx context.Context, logger logr.Logger, prowExecutionID string) JobOutcome {
	// Bound monitoring by the configured timeout while keeping a handle on the
	// caller's context, so an external cancellation (e.g. EV2/ACI sending SIGTERM
	// when the rollout is cancelled) can be told apart from our own timeout.
	parent := ctx
	monCtx, cancel := context.WithTimeout(parent, m.timeout)
	defer cancel()

	// Create ticker for polling interval
	ticker := time.NewTicker(m.pollInterval)
	defer ticker.Stop()

	// Check status immediately, then poll at intervals
	for {
		job, err := m.client.GetJobStatus(monCtx, prowExecutionID)
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
				return JobOutcome{}
			case string(prowjobs.FailureState):
				if m.gatePromotion {
					return JobOutcome{
						Err:       &JobFailedError{ProwExecutionID: prowExecutionID, JobURL: job.Status.URL},
						Retryable: true,
						JobURL:    job.Status.URL,
					}
				} else {
					logger.Info("Unexpected job state, but gating is not requested.")
					return JobOutcome{}
				}
			case string(prowjobs.ErrorState):
				if m.gatePromotion {
					return JobOutcome{
						Err:    fmt.Errorf("job %s encountered an error - check Prow status page and job logs for details: %s", prowExecutionID, job.Status.URL),
						JobURL: job.Status.URL,
					}
				} else {
					logger.Info("Unexpected job state, but gating is not requested.")
					return JobOutcome{}
				}
			case string(prowjobs.AbortedState):
				if m.gatePromotion {
					return JobOutcome{
						Err:    fmt.Errorf("job %s was aborted - this may be due to timeout or manual cancellation", prowExecutionID),
						JobURL: job.Status.URL,
					}
				} else {
					logger.Info("Unexpected job state, but gating is not requested.")
					return JobOutcome{}
				}
			}
		}

		select {
		case <-monCtx.Done():
			// Distinguish caller cancellation (rollout cancelled) from the
			// monitor's own timeout: only the former should abort the Prow job.
			if parent.Err() != nil {
				m.handleCancellation(parent, logger, prowExecutionID)
				return JobOutcome{Err: fmt.Errorf("job monitoring cancelled for job %s: %w", prowExecutionID, context.Cause(parent))}
			}
			if job != nil {
				return JobOutcome{
					Err:    fmt.Errorf("job monitoring timed out after %v - job %s may still be running, check Prow UI: %s", m.timeout, prowExecutionID, job.Status.URL),
					JobURL: job.Status.URL,
				}
			}
			return JobOutcome{Err: fmt.Errorf("job monitoring timed out after %v - job %s may still be running (unable to retrieve job status)", m.timeout, prowExecutionID)}
		case <-ticker.C:
			// Continue to next iteration
		}
	}
}

// handleCancellation makes a best-effort attempt to abort the Prow job after the
// monitoring context was cancelled by the caller (rollout cancellation). The
// abort runs on a fresh, short-lived context derived from the cancelled parent
// (values preserved, cancellation dropped) so the request can still be sent
// during the process's shutdown grace period.
func (m *Monitor) handleCancellation(parent context.Context, logger logr.Logger, prowExecutionID string) {
	if !m.abortOnCancel {
		logger.Info("Monitoring cancelled; abort-on-cancel disabled, leaving Prow job running", "prowExecutionID", prowExecutionID)
		return
	}

	logger.Info("Monitoring cancelled; handling Prow job abort", "prowExecutionID", prowExecutionID)
	// Derive a fresh, short-lived context (values preserved, cancellation dropped)
	// and re-attach the logger explicitly: the client methods extract it via
	// logr.FromContext, so the abort must not depend on the parent carrying one.
	abortCtx, cancel := context.WithTimeout(context.WithoutCancel(parent), abortTimeout)
	abortCtx = logr.NewContext(abortCtx, logger)
	defer cancel()

	if err := m.client.AbortJob(abortCtx, prowExecutionID); err != nil {
		logger.Error(err, "Failed to abort Prow job after cancellation", "prowExecutionID", prowExecutionID)
	}
}

// ExecuteAndWait submits a job once and waits for completion. It never resubmits the job
// itself. If the job's JobOutcome comes back Retryable (see JobOutcome) and this Monitor has
// allowEV2Retry set, it fetches the failed job's finished.json and, if its metadata marks it
// as safe to retry (only known-issue tests failed, see AROSLSRE-1721), returns an
// EV2RetryableError instead of the plain job-failure error. The EV2 gating step's
// pipeline.yaml matches EV2RetryableMarker via automatedRetry.errorContainsAny and re-runs
// the whole step from scratch - prow-job-executor only decides eligibility, EV2 owns the
// actual retry.
func (m *Monitor) ExecuteAndWait(ctx context.Context, logger logr.Logger, request *prowgangway.CreateJobExecutionRequest) error {
	// WaitForCompletion applies m.timeout itself; don't apply it again here too, since a
	// child context can't outlive its parent - doing so would start the whole monitoring
	// window early, before the job is even submitted.

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

	outcome := m.waitForCompletion(ctx, logger, prowExecutionID)
	if outcome.Err == nil || !m.allowEV2Retry || !outcome.Retryable {
		return outcome.Err
	}

	retry, checkErr := m.checkRetryMarker(ctx, outcome.JobURL, m.maxAutoRetryFailures)
	if checkErr != nil {
		logger.Error(checkErr, "Failed to inspect finished.json for the EV2 retry signal, failing normally", "prowExecutionID", prowExecutionID)
		return outcome.Err
	}
	if !retry {
		return outcome.Err
	}

	logger.Info("finished.json marks the run as safe to retry, failing with the EV2-retryable marker so the gating step's automatedRetry re-runs it", "prowExecutionID", prowExecutionID, "jobURL", outcome.JobURL)
	return &EV2RetryableError{Cause: outcome.Err}
}
