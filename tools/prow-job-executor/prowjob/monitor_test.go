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
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-logr/logr"

	prowgangway "sigs.k8s.io/prow/pkg/gangway"
)

// newTestServers spins up fake gangway (submit) and prow (status) endpoints.
// Each submission gets a sequentially numbered job ID ("job-1", "job-2", ...)
// whose reported state is taken from states, in order (the last entry repeats
// once exhausted).
func newTestServers(t *testing.T, states []string) (client *Client, submitCount *int32) {
	t.Helper()

	var submits int32
	var prowSrv *httptest.Server

	gangwaySrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&submits, 1)
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprintf(w, `{"id":"job-%d"}`, n)
	}))
	t.Cleanup(gangwaySrv.Close)

	prowSrv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.URL.Query().Get("prowjob")
		var idx int
		_, _ = fmt.Sscanf(id, "job-%d", &idx)
		idx-- // job-1 -> states[0]
		if idx < 0 {
			idx = 0
		}
		if idx >= len(states) {
			idx = len(states) - 1
		}
		state := states[idx]
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprintf(w, "status:\n  state: %s\n  url: https://prow.ci.openshift.org/view/gs/bucket/%s\nspec:\n  job: test-job\n", state, id)
	}))
	t.Cleanup(prowSrv.Close)

	c := NewClient("test-token", gangwaySrv.URL, prowSrv.URL)
	return c, &submits
}

func TestExecuteAndWaitFailsWithMarkerWhenEligible(t *testing.T) {
	client, submitCount := newTestServers(t, []string{"failure"})

	var markerChecks int32
	m := NewMonitor(client, time.Millisecond, time.Second, false, true, true, DefaultMaxEV2AutoRetryFailures)
	m.checkRetryMarker = func(ctx context.Context, jobURL string, maxAutoRetryFailures int) (bool, error) {
		atomic.AddInt32(&markerChecks, 1)
		if !strings.Contains(jobURL, "job-1") {
			t.Fatalf("expected marker check against the failed job, got %q", jobURL)
		}
		return true, nil
	}

	err := m.ExecuteAndWait(testContext(), logr.Discard(), &prowgangway.CreateJobExecutionRequest{})
	if err == nil {
		t.Fatal("expected an error even when the failure is retry-eligible - prow-job-executor never resubmits the job itself")
	}
	var retryable *EV2RetryableError
	if !errors.As(err, &retryable) {
		t.Fatalf("expected an EV2RetryableError so EV2's automatedRetry can match on it, got: %v", err)
	}
	if !strings.Contains(err.Error(), EV2RetryableMarker) {
		t.Fatalf("expected the error text to contain %q, got %q", EV2RetryableMarker, err.Error())
	}
	if got := atomic.LoadInt32(submitCount); got != 1 {
		t.Fatalf("expected exactly 1 submission (prow-job-executor must not resubmit), got %d", got)
	}
	if got := atomic.LoadInt32(&markerChecks); got != 1 {
		t.Fatalf("expected exactly 1 marker check, got %d", got)
	}
}

func TestExecuteAndWaitFailsPlainWhenMarkerAbsent(t *testing.T) {
	client, submitCount := newTestServers(t, []string{"failure"})

	m := NewMonitor(client, time.Millisecond, time.Second, false, true, true, DefaultMaxEV2AutoRetryFailures)
	m.checkRetryMarker = func(ctx context.Context, jobURL string, maxAutoRetryFailures int) (bool, error) {
		return false, nil
	}

	err := m.ExecuteAndWait(testContext(), logr.Discard(), &prowgangway.CreateJobExecutionRequest{})
	if err == nil {
		t.Fatal("expected the job failure to propagate when no marker is found, got nil")
	}
	var retryable *EV2RetryableError
	if errors.As(err, &retryable) {
		t.Fatalf("expected a plain error (not EV2-retryable) when the failure is not eligible, got: %v", err)
	}
	if got := atomic.LoadInt32(submitCount); got != 1 {
		t.Fatalf("expected exactly 1 submission, got %d", got)
	}
}

func TestExecuteAndWaitSkipsMarkerCheckWhenNotAllowed(t *testing.T) {
	client, submitCount := newTestServers(t, []string{"failure"})

	m := NewMonitor(client, time.Millisecond, time.Second, false, true, false, DefaultMaxEV2AutoRetryFailures)
	m.checkRetryMarker = func(ctx context.Context, jobURL string, maxAutoRetryFailures int) (bool, error) {
		t.Fatal("checkRetryMarker should not be called when allowEV2Retry is false")
		return false, nil
	}

	err := m.ExecuteAndWait(testContext(), logr.Discard(), &prowgangway.CreateJobExecutionRequest{})
	if err == nil {
		t.Fatal("expected the job failure to propagate")
	}
	if got := atomic.LoadInt32(submitCount); got != 1 {
		t.Fatalf("expected exactly 1 submission, got %d", got)
	}
}
