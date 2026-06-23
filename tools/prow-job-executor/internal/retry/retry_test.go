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

package retry

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/go-logr/logr"

	"k8s.io/apimachinery/pkg/util/wait"
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

// retryAll treats every error as transient.
func retryAll(error) bool { return true }

func TestWithValueSucceedsFirstAttempt(t *testing.T) {
	calls := 0
	got, err := WithValue(testContext(), fastBackoff(4), retryAll, func(context.Context) (int, error) {
		calls++
		return 42, nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != 42 {
		t.Fatalf("got %d, want 42", got)
	}
	if calls != 1 {
		t.Fatalf("fn called %d times, want 1", calls)
	}
}

func TestWithValueRetriesTransientThenSucceeds(t *testing.T) {
	calls := 0
	got, err := WithValue(testContext(), fastBackoff(4), retryAll, func(context.Context) (string, error) {
		calls++
		if calls < 3 {
			return "", errors.New("transient")
		}
		return "ok", nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "ok" {
		t.Fatalf("got %q, want %q", got, "ok")
	}
	if calls != 3 {
		t.Fatalf("fn called %d times, want 3", calls)
	}
}

func TestWithValueFailsFastOnNonRetryable(t *testing.T) {
	sentinel := errors.New("permanent")
	calls := 0
	_, err := WithValue(testContext(), fastBackoff(4), func(error) bool { return false }, func(context.Context) (int, error) {
		calls++
		return 0, sentinel
	})
	if !errors.Is(err, sentinel) {
		t.Fatalf("error = %v, want it to wrap sentinel", err)
	}
	if calls != 1 {
		t.Fatalf("fn called %d times, want 1 (no retries on permanent error)", calls)
	}
	// A permanent error must propagate unchanged, not behind the "retry budget" wrapper.
	if strings.Contains(err.Error(), "retry budget exhausted") {
		t.Fatalf("permanent error should propagate as-is, got %q", err.Error())
	}
}

func TestWithValueWrapsLastErrorWhenExhausted(t *testing.T) {
	sentinel := errors.New("still failing")
	calls := 0
	_, err := WithValue(testContext(), fastBackoff(3), retryAll, func(context.Context) (int, error) {
		calls++
		return 0, sentinel
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, sentinel) {
		t.Fatalf("error = %v, want it to wrap the last transient error", err)
	}
	if !strings.Contains(err.Error(), "retry budget exhausted") {
		t.Fatalf("exhausted error %q should mention the retry budget", err.Error())
	}
	if calls != 3 {
		t.Fatalf("fn called %d times, want 3", calls)
	}
}

// TestWithValueContextCanceledBeforeStart verifies a context cancelled before the
// first attempt surfaces as-is and never invokes fn (ExponentialBackoffWithContext
// checks the context before the first condition call).
func TestWithValueContextCanceledBeforeStart(t *testing.T) {
	ctx, cancel := context.WithCancel(testContext())
	cancel()

	calls := 0
	_, err := WithValue(ctx, fastBackoff(4), retryAll, func(ctx context.Context) (int, error) {
		calls++
		return 0, ctx.Err()
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
	if strings.Contains(err.Error(), "retry budget exhausted") {
		t.Fatalf("context error should surface as-is, got %q", err.Error())
	}
	if calls != 0 {
		t.Fatalf("fn called %d times, want 0 (context checked before first attempt)", calls)
	}
}

// TestWithValueContextCanceledDuringCall verifies that when the parent context is
// cancelled while fn is running, the next iteration fails fast on the context error
// without logging a "will retry" or wrapping it as an exhausted-budget error — even
// though fn returned an otherwise-retryable error.
func TestWithValueContextCanceledDuringCall(t *testing.T) {
	ctx, cancel := context.WithCancel(testContext())

	calls := 0
	_, err := WithValue(ctx, fastBackoff(4), retryAll, func(ctx context.Context) (int, error) {
		calls++
		cancel()                          // cancel the parent mid-call
		return 0, errors.New("transient") // a normally-retryable error
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
	if strings.Contains(err.Error(), "retry budget exhausted") {
		t.Fatalf("cancelled context should not be wrapped as exhausted budget, got %q", err.Error())
	}
	if calls != 1 {
		t.Fatalf("fn called %d times, want 1 (no retry after cancellation)", calls)
	}
}
