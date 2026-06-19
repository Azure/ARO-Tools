package helm

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/go-logr/logr/testr"

	kapierrors "k8s.io/apimachinery/pkg/api/errors"
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
}
