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

package prowjobexecutor

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/go-logr/logr"

	"k8s.io/apimachinery/pkg/util/wait"

	"github.com/Azure/ARO-Tools/tools/prow-job-executor/internal/retry"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
)

// fastBackoff returns a backoff with negligible delays so retry tests run quickly
// while still exercising a deterministic number of attempts. No Cap is set so the
// attempt count is exactly Steps (apimachinery's Cap would also halt retries early).
func fastBackoff(steps int) wait.Backoff {
	return wait.Backoff{
		Duration: time.Millisecond,
		Factor:   2.0,
		Jitter:   0.0,
		Steps:    steps,
	}
}

func testContext() context.Context {
	return logr.NewContext(context.Background(), logr.Discard())
}

func responseError(statusCode int) error {
	return &azcore.ResponseError{StatusCode: statusCode}
}

func TestIsRetryableKeyVaultError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{name: "key vault 429 is retryable", err: responseError(http.StatusTooManyRequests), want: true},
		{name: "key vault 500 is retryable", err: responseError(http.StatusInternalServerError), want: true},
		{name: "key vault 503 is retryable", err: responseError(http.StatusServiceUnavailable), want: true},
		{name: "key vault 401 is not retryable", err: responseError(http.StatusUnauthorized), want: false},
		{name: "key vault 403 is not retryable", err: responseError(http.StatusForbidden), want: false},
		{name: "key vault 404 is not retryable", err: responseError(http.StatusNotFound), want: false},
		{name: "key vault 400 is not retryable", err: responseError(http.StatusBadRequest), want: false},
		{name: "wrapped key vault 403 is not retryable", err: fmt.Errorf("get secret: %w", responseError(http.StatusForbidden)), want: false},
		{name: "wrapped key vault 503 is retryable", err: fmt.Errorf("get secret: %w", responseError(http.StatusServiceUnavailable)), want: true},
		{name: "imds eof credential error is retryable", err: errors.New("ManagedIdentityCredential: Get \"http://169.254.169.254/metadata/identity/oauth2/token\": EOF"), want: true},
		{name: "generic non-response error is retryable", err: errors.New("boom"), want: true},
		{name: "permanent local error is not retryable", err: fmt.Errorf("failed to create Key Vault client: %w: %w", errors.New("bad uri"), errPermanentKeyVaultLookup), want: false},
		{name: "wrapped permanent local error is not retryable", err: fmt.Errorf("lookup failed: %w", fmt.Errorf("secret has no value: %w", errPermanentKeyVaultLookup)), want: false},
		{name: "context canceled is not retryable", err: context.Canceled, want: false},
		{name: "context deadline exceeded is not retryable", err: context.DeadlineExceeded, want: false},
		{name: "wrapped context canceled is not retryable", err: fmt.Errorf("get secret: %w", context.Canceled), want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isRetryableKeyVaultError(tt.err); got != tt.want {
				t.Fatalf("isRetryableKeyVaultError(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}

func TestRetryProwTokenLookup(t *testing.T) {
	tests := []struct {
		name string
		// errs returned in order before "success"; an entry of nil means success.
		errs      []error
		wantToken string
		wantErr   bool
		wantCalls int
	}{
		{
			name:      "succeeds on first attempt",
			errs:      []error{nil},
			wantToken: "secret-value",
			wantCalls: 1,
		},
		{
			name:      "retries transient imds eof then succeeds",
			errs:      []error{errors.New("ManagedIdentityCredential: ...: EOF"), nil},
			wantToken: "secret-value",
			wantCalls: 2,
		},
		{
			name:      "retries key vault 503 then succeeds",
			errs:      []error{responseError(http.StatusServiceUnavailable), nil},
			wantToken: "secret-value",
			wantCalls: 2,
		},
		{
			name:      "permanent 403 fails fast without retry",
			errs:      []error{responseError(http.StatusForbidden)},
			wantErr:   true,
			wantCalls: 1,
		},
		{
			name:      "permanent 404 fails fast without retry",
			errs:      []error{responseError(http.StatusNotFound)},
			wantErr:   true,
			wantCalls: 1,
		},
		{
			name:      "permanent local error fails fast without retry",
			errs:      []error{fmt.Errorf("secret has no value: %w", errPermanentKeyVaultLookup)},
			wantErr:   true,
			wantCalls: 1,
		},
		{
			name:      "persistent transient error exhausts retries",
			errs:      []error{responseError(http.StatusTooManyRequests)}, // repeats until steps exhausted
			wantErr:   true,
			wantCalls: 4,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			calls := 0
			fetch := func(ctx context.Context) (string, error) {
				idx := calls
				calls++
				// Repeat the last error once the scripted slice is exhausted, to
				// model a persistently-failing transient condition.
				if idx >= len(tt.errs) {
					idx = len(tt.errs) - 1
				}
				if err := tt.errs[idx]; err != nil {
					return "", err
				}
				return "secret-value", nil
			}

			token, err := retry.WithValue(testContext(), fastBackoff(4), isRetryableKeyVaultError, fetch)

			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil (token %q)", token)
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if token != tt.wantToken {
					t.Fatalf("token = %q, want %q", token, tt.wantToken)
				}
			}

			if calls != tt.wantCalls {
				t.Fatalf("fetch called %d times, want %d", calls, tt.wantCalls)
			}
		})
	}
}

// TestRetryProwTokenLookupContextCanceled verifies that a cancelled parent context
// fails fast (no retries) and the returned error is the context error itself, not a
// "...after retries" wrapper hiding it.
func TestRetryProwTokenLookupContextCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(testContext())
	cancel()

	calls := 0
	fetch := func(ctx context.Context) (string, error) {
		calls++
		return "", ctx.Err()
	}

	_, err := retry.WithValue(ctx, fastBackoff(4), isRetryableKeyVaultError, fetch)
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want it to wrap context.Canceled", err)
	}
}
