package helm

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/go-logr/logr/testr"

	helmrelease "helm.sh/helm/v4/pkg/release"
	helmreleasecommon "helm.sh/helm/v4/pkg/release/common"
	helmreleasev1 "helm.sh/helm/v4/pkg/release/v1"
)

func pendingRelease(name, namespace string, revision int, status helmreleasecommon.Status, lastDeployed time.Time) helmrelease.Releaser {
	return &helmreleasev1.Release{
		Name:      name,
		Namespace: namespace,
		Version:   revision,
		Info: &helmreleasev1.Info{
			Status:       status,
			LastDeployed: lastDeployed,
		},
	}
}

func TestCheckForStaleReleaseLock(t *testing.T) {
	const threshold = 15 * time.Minute
	now := time.Now()

	tests := []struct {
		name        string
		versions    []helmrelease.Releaser
		wantStale   bool
		wantRelease string
		wantNS      string
		wantRev     int
		wantStatus  string
		wantSecret  string
	}{
		{
			name: "stale pending-upgrade fails fast",
			versions: []helmrelease.Releaser{
				pendingRelease("backend", "aro-hcp", 7, helmreleasecommon.StatusPendingUpgrade, now.Add(-42*time.Minute)),
			},
			wantStale:   true,
			wantRelease: "backend",
			wantNS:      "aro-hcp",
			wantRev:     7,
			wantStatus:  "pending-upgrade",
			wantSecret:  "sh.helm.release.v1.backend.v7",
		},
		{
			name: "stale pending-install fails fast",
			versions: []helmrelease.Releaser{
				pendingRelease("frontend", "aro-hcp", 1, helmreleasecommon.StatusPendingInstall, now.Add(-1*time.Hour)),
			},
			wantStale:   true,
			wantRelease: "frontend",
			wantNS:      "aro-hcp",
			wantRev:     1,
			wantStatus:  "pending-install",
			wantSecret:  "sh.helm.release.v1.frontend.v1",
		},
		{
			name: "normal deployed history does not trigger",
			versions: []helmrelease.Releaser{
				pendingRelease("backend", "aro-hcp", 7, helmreleasecommon.StatusDeployed, now.Add(-42*time.Minute)),
			},
			wantStale: false,
		},
		{
			name: "fresh pending operation within threshold does not trigger",
			versions: []helmrelease.Releaser{
				pendingRelease("backend", "aro-hcp", 8, helmreleasecommon.StatusPendingUpgrade, now.Add(-2*time.Minute)),
			},
			wantStale: false,
		},
		{
			name:      "empty history does not trigger",
			versions:  nil,
			wantStale: false,
		},
		{
			name: "only the latest revision is considered",
			versions: []helmrelease.Releaser{
				pendingRelease("backend", "aro-hcp", 6, helmreleasecommon.StatusPendingUpgrade, now.Add(-1*time.Hour)),
				pendingRelease("backend", "aro-hcp", 7, helmreleasecommon.StatusDeployed, now.Add(-30*time.Minute)),
			},
			wantStale: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			logger := testr.New(t)
			err := checkForStaleReleaseLock(logger, threshold, tc.versions)

			if !tc.wantStale {
				if err != nil {
					t.Fatalf("expected no stale-lock error, got: %v", err)
				}
				return
			}

			if err == nil {
				t.Fatalf("expected a stale-lock error, got nil")
			}
			var staleErr *StaleReleaseLockError
			if !errors.As(err, &staleErr) {
				t.Fatalf("expected *StaleReleaseLockError, got %T: %v", err, err)
			}
			if staleErr.ReleaseName != tc.wantRelease {
				t.Errorf("release name: want %q, got %q", tc.wantRelease, staleErr.ReleaseName)
			}
			if staleErr.Namespace != tc.wantNS {
				t.Errorf("namespace: want %q, got %q", tc.wantNS, staleErr.Namespace)
			}
			if staleErr.Revision != tc.wantRev {
				t.Errorf("revision: want %d, got %d", tc.wantRev, staleErr.Revision)
			}
			if staleErr.Status != tc.wantStatus {
				t.Errorf("status: want %q, got %q", tc.wantStatus, staleErr.Status)
			}
			if staleErr.SecretName != tc.wantSecret {
				t.Errorf("secret name: want %q, got %q", tc.wantSecret, staleErr.SecretName)
			}
			if staleErr.Threshold != threshold {
				t.Errorf("threshold: want %s, got %s", threshold, staleErr.Threshold)
			}
			if staleErr.Age < threshold {
				t.Errorf("age %s should be >= threshold %s", staleErr.Age, threshold)
			}
		})
	}
}

func TestStaleReleaseLockErrorMessage(t *testing.T) {
	err := &StaleReleaseLockError{
		ReleaseName: "backend",
		Namespace:   "aro-hcp",
		Revision:    7,
		Status:      "pending-upgrade",
		Age:         42 * time.Minute,
		Threshold:   15 * time.Minute,
		SecretName:  "sh.helm.release.v1.backend.v7",
	}

	msg := err.Error()
	for _, want := range []string{
		"backend",
		"aro-hcp",
		"pending-upgrade",
		"revision 7",
		"sh.helm.release.v1.backend.v7",
		"kubectl",
		"delete secret",
	} {
		if !strings.Contains(msg, want) {
			t.Errorf("error message missing %q; full message:\n%s", want, msg)
		}
	}
}
