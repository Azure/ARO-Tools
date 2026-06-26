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

// abortTimeout bounds the best-effort abort issued when monitoring is cancelled.
// It must stay well within the process's shutdown grace period so the request can
// be sent before the container is killed.
const abortTimeout = 30 * time.Second

// Monitor handles job execution and monitoring
type Monitor struct {
	client        *Client
	pollInterval  time.Duration
	timeout       time.Duration
	dryRun        bool
	gatePromotion bool
	abortOnCancel bool
}

// NewMonitor creates a new job monitor with the specified polling interval and timeout.
func NewMonitor(client *Client, pollInterval, timeout time.Duration, dryRun, gatePromotion, abortOnCancel bool) *Monitor {
	return &Monitor{
		client:        client,
		pollInterval:  pollInterval,
		timeout:       timeout,
		dryRun:        dryRun,
		gatePromotion: gatePromotion,
		abortOnCancel: abortOnCancel,
	}
}

// WaitForCompletion polls job status until completion
func (m *Monitor) WaitForCompletion(ctx context.Context, logger logr.Logger, prowExecutionID string) error {
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
				return nil
			case string(prowjobs.FailureState):
				if m.gatePromotion {
					return fmt.Errorf("job %s failed - check the Prow UI for detailed logs: %s", prowExecutionID, job.Status.URL)
				} else {
					logger.Error(err, "Unexpected job state, but gating is not requested.")
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
		case <-monCtx.Done():
			// Distinguish caller cancellation (rollout cancelled) from the
			// monitor's own timeout: only the former should abort the Prow job.
			if parent.Err() != nil {
				m.handleCancellation(parent, logger, prowExecutionID)
				return fmt.Errorf("job monitoring cancelled for job %s: %w", prowExecutionID, context.Cause(parent))
			}
			if job != nil {
				return fmt.Errorf("job monitoring timed out after %v - job %s may still be running, check Prow UI: %s", m.timeout, prowExecutionID, job.Status.URL)
			}
			return fmt.Errorf("job monitoring timed out after %v - job %s may still be running (unable to retrieve job status)", m.timeout, prowExecutionID)
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
	abortCtx, cancel := context.WithTimeout(context.WithoutCancel(parent), abortTimeout)
	defer cancel()

	if err := m.client.AbortJob(abortCtx, prowExecutionID); err != nil {
		logger.Error(err, "Failed to abort Prow job after cancellation", "prowExecutionID", prowExecutionID)
	}
}

// ExecuteAndWait submits a job and waits for completion
func (m *Monitor) ExecuteAndWait(ctx context.Context, logger logr.Logger, request *prowgangway.CreateJobExecutionRequest) error {
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

	// Wait for completion using shared logic
	return m.WaitForCompletion(ctx, logger, prowExecutionID)
}
