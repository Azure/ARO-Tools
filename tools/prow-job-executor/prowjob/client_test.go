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
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-logr/logr"

	"k8s.io/apimachinery/pkg/util/wait"

	prowgangway "sigs.k8s.io/prow/pkg/gangway"
)

// fastBackoffSteps is the deterministic attempt budget used by retry tests.
const fastBackoffSteps = 5

// fastBackoff returns a backoff with negligible delays so retry tests run quickly
// while still exercising a deterministic number of attempts. No Cap is set so the
// attempt count is exactly Steps (apimachinery's Cap would also halt retries early).
func fastBackoff() wait.Backoff {
	return wait.Backoff{
		Duration: time.Millisecond,
		Factor:   2.0,
		Jitter:   0.0,
		Steps:    fastBackoffSteps,
	}
}

func testContext() context.Context {
	return logr.NewContext(context.Background(), logr.Discard())
}

func TestSubmitJobRetry(t *testing.T) {
	const successBody = `{"id":"job-exec-123"}`

	tests := []struct {
		name string
		// statuses returned in order; the last status repeats once exhausted.
		statuses        []int
		wantID          string
		wantErr         bool
		wantErrContains string
		wantAttempts    int32
	}{
		{
			name:         "success on first attempt",
			statuses:     []int{http.StatusOK},
			wantID:       "job-exec-123",
			wantAttempts: 1,
		},
		{
			name:         "429 then success",
			statuses:     []int{http.StatusTooManyRequests, http.StatusOK},
			wantID:       "job-exec-123",
			wantAttempts: 2,
		},
		{
			name:         "503 then success",
			statuses:     []int{http.StatusServiceUnavailable, http.StatusOK},
			wantID:       "job-exec-123",
			wantAttempts: 2,
		},
		{
			name:         "500 then success",
			statuses:     []int{http.StatusInternalServerError, http.StatusOK},
			wantID:       "job-exec-123",
			wantAttempts: 2,
		},
		{
			name:            "persistent 429 exhausts retries",
			statuses:        []int{http.StatusTooManyRequests},
			wantErr:         true,
			wantErrContains: "429",
			wantAttempts:    fastBackoffSteps,
		},
		{
			name:            "persistent 502 exhausts retries",
			statuses:        []int{http.StatusBadGateway},
			wantErr:         true,
			wantErrContains: "502",
			wantAttempts:    fastBackoffSteps,
		},
		{
			name:            "401 is not retried",
			statuses:        []int{http.StatusUnauthorized},
			wantErr:         true,
			wantErrContains: "401",
			wantAttempts:    1,
		},
		{
			name:            "403 is not retried",
			statuses:        []int{http.StatusForbidden},
			wantErr:         true,
			wantErrContains: "403",
			wantAttempts:    1,
		},
		{
			name:            "400 is not retried",
			statuses:        []int{http.StatusBadRequest},
			wantErr:         true,
			wantErrContains: "400",
			wantAttempts:    1,
		},
		{
			name:            "404 is not retried",
			statuses:        []int{http.StatusNotFound},
			wantErr:         true,
			wantErrContains: "404",
			wantAttempts:    1,
		},
		{
			name:            "409 is not retried",
			statuses:        []int{http.StatusConflict},
			wantErr:         true,
			wantErrContains: "409",
			wantAttempts:    1,
		},
		{
			name:            "422 is not retried",
			statuses:        []int{http.StatusUnprocessableEntity},
			wantErr:         true,
			wantErrContains: "422",
			wantAttempts:    1,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var attempts int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				n := atomic.AddInt32(&attempts, 1)
				idx := int(n) - 1
				if idx >= len(tc.statuses) {
					idx = len(tc.statuses) - 1
				}
				status := tc.statuses[idx]
				w.WriteHeader(status)
				if status == http.StatusOK {
					_, _ = w.Write([]byte(successBody))
				} else {
					_, _ = w.Write([]byte(http.StatusText(status)))
				}
			}))
			defer srv.Close()

			c := NewClient("test-token", srv.URL, srv.URL)
			c.submitBackoff = fastBackoff()

			id, err := c.SubmitJob(testContext(), &prowgangway.CreateJobExecutionRequest{})

			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil (id=%q)", id)
				}
				if tc.wantErrContains != "" && !strings.Contains(err.Error(), tc.wantErrContains) {
					t.Fatalf("error %q does not contain %q", err.Error(), tc.wantErrContains)
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if id != tc.wantID {
					t.Fatalf("got id %q, want %q", id, tc.wantID)
				}
			}

			if got := atomic.LoadInt32(&attempts); got != tc.wantAttempts {
				t.Fatalf("got %d attempts, want %d", got, tc.wantAttempts)
			}
		})
	}
}

// TestSubmitJobNetworkErrorRetries verifies transport errors (server unreachable)
// are treated as transient and retried up to the attempt budget.
func TestSubmitJobNetworkErrorRetries(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	url := srv.URL
	srv.Close() // ensure connections are refused

	c := NewClient("test-token", url, url)
	c.submitBackoff = fastBackoff()

	_, err := c.SubmitJob(testContext(), &prowgangway.CreateJobExecutionRequest{})
	if err == nil {
		t.Fatal("expected error for unreachable server, got nil")
	}
}

func TestIsRetryableError(t *testing.T) {
	tests := []struct {
		name      string
		err       error
		retryable bool
	}{
		{"429", &httpStatusError{statusCode: http.StatusTooManyRequests}, true},
		{"500", &httpStatusError{statusCode: http.StatusInternalServerError}, true},
		{"503", &httpStatusError{statusCode: http.StatusServiceUnavailable}, true},
		{"504", &httpStatusError{statusCode: http.StatusGatewayTimeout}, true},
		{"400", &httpStatusError{statusCode: http.StatusBadRequest}, false},
		{"401", &httpStatusError{statusCode: http.StatusUnauthorized}, false},
		{"403", &httpStatusError{statusCode: http.StatusForbidden}, false},
		{"404", &httpStatusError{statusCode: http.StatusNotFound}, false},
		{"409", &httpStatusError{statusCode: http.StatusConflict}, false},
		{"non-http error", context.Canceled, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := isRetryableError(tc.err); got != tc.retryable {
				t.Errorf("isRetryableError(%v) = %v, want %v", tc.err, got, tc.retryable)
			}
		})
	}
}

func TestIsRetryableStatusCode(t *testing.T) {
	// isRetryableStatusCode backs GetJobStatus, where everything except 401/403 is
	// retried (a freshly submitted job's status may 404 until it propagates).
	tests := []struct {
		status    int
		retryable bool
	}{
		{http.StatusTooManyRequests, true},
		{http.StatusServiceUnavailable, true},
		{http.StatusInternalServerError, true},
		{http.StatusNotFound, true},
		{http.StatusUnauthorized, false},
		{http.StatusForbidden, false},
	}
	for _, tc := range tests {
		if got := isRetryableStatusCode(tc.status); got != tc.retryable {
			t.Errorf("isRetryableStatusCode(%d) = %v, want %v", tc.status, got, tc.retryable)
		}
	}
}
