package helm

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/go-logr/logr/testr"

	kapierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/util/wait"
)

func TestIsTransientCredentialError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "nil is not transient",
			err:  nil,
			want: false,
		},
		{
			name: "credential plugin failure has no API status, is transient",
			err:  fmt.Errorf(`failed to apply namespace acrpull: Patch "https://x.azmk8s.io/api/v1/namespaces/acrpull": getting credentials: exec: executable kubelogin failed with exit code 1`),
			want: true,
		},
		{
			name: "plain transport error is transient",
			err:  errors.New("connection aborted"),
			want: true,
		},
		{
			name: "unauthorized is a genuine auth failure, not transient",
			err:  kapierrors.NewUnauthorized("token invalid"),
			want: false,
		},
		{
			name: "service unavailable is transient",
			err:  kapierrors.NewServiceUnavailable("apiserver overloaded"),
			want: true,
		},
		{
			name: "too many requests is transient",
			err:  kapierrors.NewTooManyRequests("slow down", 1),
			want: true,
		},
		{
			name: "internal error is transient",
			err:  kapierrors.NewInternalError(errors.New("boom")),
			want: true,
		},
		{
			name: "wrapped unauthorized is still not transient",
			err:  fmt.Errorf("apply failed: %w", kapierrors.NewUnauthorized("token invalid")),
			want: false,
		},
		{
			name: "context canceled is not transient",
			err:  context.Canceled,
			want: false,
		},
		{
			name: "context deadline exceeded is not transient",
			err:  context.DeadlineExceeded,
			want: false,
		},
		{
			name: "wrapped context canceled is not transient",
			err:  fmt.Errorf("apply namespace: %w", context.Canceled),
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isTransientCredentialError(tt.err); got != tt.want {
				t.Fatalf("isTransientCredentialError(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}

func TestRetryOnTransientCredentialError(t *testing.T) {
	logger := testr.New(t)

	// Use a fast, no-sleep backoff so retries do not consume real wall-clock time.
	restore := transientCredentialErrorBackoff
	transientCredentialErrorBackoff = func() wait.Backoff {
		return wait.Backoff{Duration: time.Microsecond, Factor: 1.0, Steps: 5}
	}
	t.Cleanup(func() { transientCredentialErrorBackoff = restore })

	t.Run("retries transient failures then succeeds", func(t *testing.T) {
		calls := 0
		err := retryOnTransientCredentialError(context.Background(), logger, "apply namespace acrpull", func(context.Context) error {
			calls++
			if calls < 3 {
				return errors.New("getting credentials: exec: executable kubelogin failed with exit code 1")
			}
			return nil
		})
		if err != nil {
			t.Fatalf("expected success, got %v", err)
		}
		if calls != 3 {
			t.Fatalf("expected 3 attempts, got %d", calls)
		}
	})

	t.Run("fails fast on deterministic error", func(t *testing.T) {
		calls := 0
		want := kapierrors.NewUnauthorized("token invalid")
		err := retryOnTransientCredentialError(context.Background(), logger, "apply namespace acrpull", func(context.Context) error {
			calls++
			return want
		})
		if !errors.Is(err, want) {
			t.Fatalf("expected unauthorized error, got %v", err)
		}
		if calls != 1 {
			t.Fatalf("expected exactly 1 attempt, got %d", calls)
		}
	})

	t.Run("preserves context cancellation instead of masking it", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		calls := 0
		err := retryOnTransientCredentialError(ctx, logger, "apply namespace acrpull", func(context.Context) error {
			calls++
			return errors.New("getting credentials: exec: executable kubelogin failed with exit code 1")
		})
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("expected context.Canceled, got %v", err)
		}
	})
}
