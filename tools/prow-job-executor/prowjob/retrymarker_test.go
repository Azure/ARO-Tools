// Copyright 2026 Microsoft Corporation
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
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestFinishedJSONURLFromViewURL(t *testing.T) {
	tests := []struct {
		name    string
		viewURL string
		want    string
		wantErr bool
	}{
		{
			name:    "typical prow deck view URL",
			viewURL: "https://prow.ci.openshift.org/view/gs/origin-ci-test/logs/branch-ci-Azure-ARO-HCP-main-e2e-stage-e2e-parallel/12345",
			want:    "https://storage.googleapis.com/origin-ci-test/logs/branch-ci-Azure-ARO-HCP-main-e2e-stage-e2e-parallel/12345/finished.json",
		},
		{
			name:    "trailing slash is trimmed",
			viewURL: "https://prow.ci.openshift.org/view/gs/origin-ci-test/logs/some-job/1/",
			want:    "https://storage.googleapis.com/origin-ci-test/logs/some-job/1/finished.json",
		},
		{
			name:    "empty URL is an error",
			viewURL: "",
			wantErr: true,
		},
		{
			name:    "missing /view/gs/ prefix is an error",
			viewURL: "https://prow.ci.openshift.org/something-else/origin-ci-test/logs/some-job/1",
			wantErr: true,
		},
		{
			name:    "empty GCS path after /view/gs/ prefix is an error",
			viewURL: "https://prow.ci.openshift.org/view/gs/",
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := finishedJSONURLFromViewURL(tc.viewURL)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got URL %q", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestFetchFinishedJSONAllowsRetry(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		body       string
		want       bool
		wantErr    bool
	}{
		{
			name:       "single allow-retry failure qualifies",
			statusCode: http.StatusOK,
			body:       `{"timestamp":1,"passed":false,"result":"FAILURE","metadata":{"ev2-failed-tests":["spec A"],"ev2-allow-retry-tests":["spec A"],"pod":"abc"}}`,
			want:       true,
		},
		{
			name:       "failures at the cap still qualify",
			statusCode: http.StatusOK,
			body:       `{"timestamp":1,"passed":false,"result":"FAILURE","metadata":{"ev2-failed-tests":["spec A","spec B"],"ev2-allow-retry-tests":["spec A","spec B"]}}`,
			want:       true,
		},
		{
			name:       "one failure over the cap disqualifies",
			statusCode: http.StatusOK,
			body:       `{"timestamp":1,"passed":false,"result":"FAILURE","metadata":{"ev2-failed-tests":["spec A","spec B","spec C"],"ev2-allow-retry-tests":["spec A","spec B","spec C"]}}`,
			want:       false,
		},
		{
			name:       "an unlabeled failure disqualifies the whole run",
			statusCode: http.StatusOK,
			body:       `{"timestamp":1,"passed":false,"result":"FAILURE","metadata":{"ev2-failed-tests":["spec A","spec B"],"ev2-allow-retry-tests":["spec A"]}}`,
			want:       false,
		},
		{
			name:       "metadata keys absent means no failures reported",
			statusCode: http.StatusOK,
			body:       `{"timestamp":1,"passed":true,"result":"SUCCESS","metadata":{"pod":"abc"}}`,
			want:       false,
		},
		{
			name:       "empty failed-tests list is a clean run",
			statusCode: http.StatusOK,
			body:       `{"timestamp":1,"passed":true,"result":"SUCCESS","metadata":{"ev2-failed-tests":[],"ev2-allow-retry-tests":[]}}`,
			want:       false,
		},
		{
			name:       "no metadata object at all",
			statusCode: http.StatusOK,
			body:       `{"timestamp":1,"passed":false,"result":"FAILURE"}`,
			want:       false,
		},
		{
			name:       "metadata key wrong type is treated as absent",
			statusCode: http.StatusOK,
			body:       `{"timestamp":1,"passed":false,"result":"FAILURE","metadata":{"ev2-failed-tests":"spec A"}}`,
			want:       false,
		},
		{
			name:       "non-200 status is an error",
			statusCode: http.StatusNotFound,
			body:       "not found",
			wantErr:    true,
		},
		{
			name:       "invalid JSON is an error",
			statusCode: http.StatusOK,
			body:       `{not json`,
			wantErr:    true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.statusCode)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer srv.Close()

			got, err := fetchFinishedJSONAllowsRetry(testContext(), srv.URL, DefaultMaxEV2AutoRetryFailures)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got %v", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestFetchFinishedJSONAllowsRetryRejectsOversizedBody(t *testing.T) {
	// A finished.json far larger than any real one should still be read (up to
	// the cap) without hanging or OOMing, and simply fail to parse as JSON
	// once truncated - proving the size cap is actually enforced.
	huge := `{"metadata":{"padding":"` + strings.Repeat("x", maxFinishedJSONBytes+1024) + `","ev2-failed-tests":["spec A"]}}`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(huge))
	}))
	defer srv.Close()

	_, err := fetchFinishedJSONAllowsRetry(testContext(), srv.URL, DefaultMaxEV2AutoRetryFailures)
	if err == nil {
		t.Fatal("expected an error from truncated/invalid JSON, got nil")
	}
}

func TestEV2RetryEligible(t *testing.T) {
	for _, tc := range []struct {
		name       string
		failed     []string
		allowRetry []string
		maxFailure int
		want       bool
	}{
		{
			name:       "clean run does not qualify",
			maxFailure: 2,
			want:       false,
		},
		{
			name:       "single labeled failure qualifies",
			failed:     []string{"spec A"},
			allowRetry: []string{"spec A"},
			maxFailure: 2,
			want:       true,
		},
		{
			name:       "failures at the cap still qualify",
			failed:     []string{"spec A", "spec B"},
			allowRetry: []string{"spec A", "spec B"},
			maxFailure: 2,
			want:       true,
		},
		{
			name:       "one failure over the cap disqualifies",
			failed:     []string{"spec A", "spec B", "spec C"},
			allowRetry: []string{"spec A", "spec B", "spec C"},
			maxFailure: 2,
			want:       false,
		},
		{
			name:       "an unlabeled failure disqualifies the whole run",
			failed:     []string{"spec A", "spec B"},
			allowRetry: []string{"spec A"},
			maxFailure: 2,
			want:       false,
		},
		{
			name:       "a lone unlabeled failure disqualifies",
			failed:     []string{"spec A"},
			allowRetry: nil,
			maxFailure: 2,
			want:       false,
		},
		{
			name:       "a lower configured cap tightens eligibility",
			failed:     []string{"spec A", "spec B"},
			allowRetry: []string{"spec A", "spec B"},
			maxFailure: 1,
			want:       false,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := ev2RetryEligible(tc.failed, tc.allowRetry, tc.maxFailure); got != tc.want {
				t.Fatalf("ev2RetryEligible(%v, %v, %d) = %v, want %v", tc.failed, tc.allowRetry, tc.maxFailure, got, tc.want)
			}
		})
	}
}
