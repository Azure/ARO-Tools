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

func TestExecuteAndWaitRetriesOnceWhenMarkerFound(t *testing.T) {
	client, submitCount := newTestServers(t, []string{"failure", "success"})

	var markerChecks int32
	m := NewMonitor(client, time.Millisecond, time.Second, false, true, true)
	m.checkRetryMarker = func(ctx context.Context, jobURL string) (bool, error) {
		atomic.AddInt32(&markerChecks, 1)
		if !strings.Contains(jobURL, "job-1") {
			t.Fatalf("expected marker check against the first (failed) job, got %q", jobURL)
		}
		return true, nil
	}

	err := m.ExecuteAndWait(testContext(), logr.Discard(), &prowgangway.CreateJobExecutionRequest{})
	if err != nil {
		t.Fatalf("expected the retried job to succeed, got error: %v", err)
	}
	if got := atomic.LoadInt32(submitCount); got != 2 {
		t.Fatalf("expected 2 submissions (original + 1 retry), got %d", got)
	}
	if got := atomic.LoadInt32(&markerChecks); got != 1 {
		t.Fatalf("expected exactly 1 marker check, got %d", got)
	}
}

func TestExecuteAndWaitDoesNotRetryWhenMarkerAbsent(t *testing.T) {
	client, submitCount := newTestServers(t, []string{"failure", "success"})

	m := NewMonitor(client, time.Millisecond, time.Second, false, true, true)
	m.checkRetryMarker = func(ctx context.Context, jobURL string) (bool, error) {
		return false, nil
	}

	err := m.ExecuteAndWait(testContext(), logr.Discard(), &prowgangway.CreateJobExecutionRequest{})
	if err == nil {
		t.Fatal("expected the job failure to propagate when no marker is found, got nil")
	}
	if got := atomic.LoadInt32(submitCount); got != 1 {
		t.Fatalf("expected exactly 1 submission (no retry), got %d", got)
	}
}

func TestExecuteAndWaitDoesNotRetryTwice(t *testing.T) {
	client, submitCount := newTestServers(t, []string{"failure", "failure"})

	var markerChecks int32
	m := NewMonitor(client, time.Millisecond, time.Second, false, true, true)
	m.checkRetryMarker = func(ctx context.Context, jobURL string) (bool, error) {
		atomic.AddInt32(&markerChecks, 1)
		return true, nil
	}

	err := m.ExecuteAndWait(testContext(), logr.Discard(), &prowgangway.CreateJobExecutionRequest{})
	if err == nil {
		t.Fatal("expected an error since the retried job also failed")
	}
	if got := atomic.LoadInt32(submitCount); got != 2 {
		t.Fatalf("expected exactly 2 submissions (original + the single retry, no further retry), got %d", got)
	}
	if got := atomic.LoadInt32(&markerChecks); got != 1 {
		t.Fatalf("expected exactly 1 marker check (the retried attempt has allowEV2Retry disabled), got %d", got)
	}
}

func TestExecuteAndWaitSkipsRetryWhenNotAllowed(t *testing.T) {
	client, submitCount := newTestServers(t, []string{"failure"})

	m := NewMonitor(client, time.Millisecond, time.Second, false, true, false)
	m.checkRetryMarker = func(ctx context.Context, jobURL string) (bool, error) {
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
