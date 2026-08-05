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
			name:       "metadata key true",
			statusCode: http.StatusOK,
			body:       `{"timestamp":1,"passed":false,"result":"FAILURE","metadata":{"ev2-retry-allowed":true,"pod":"abc"}}`,
			want:       true,
		},
		{
			name:       "metadata key absent",
			statusCode: http.StatusOK,
			body:       `{"timestamp":1,"passed":false,"result":"FAILURE","metadata":{"pod":"abc"}}`,
			want:       false,
		},
		{
			name:       "metadata key false",
			statusCode: http.StatusOK,
			body:       `{"timestamp":1,"passed":false,"result":"FAILURE","metadata":{"ev2-retry-allowed":false}}`,
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
			body:       `{"timestamp":1,"passed":false,"result":"FAILURE","metadata":{"ev2-retry-allowed":"true"}}`,
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

			got, err := fetchFinishedJSONAllowsRetry(testContext(), srv.URL)
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
	huge := `{"metadata":{"padding":"` + strings.Repeat("x", maxFinishedJSONBytes+1024) + `","ev2-retry-allowed":true}}`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(huge))
	}))
	defer srv.Close()

	_, err := fetchFinishedJSONAllowsRetry(testContext(), srv.URL)
	if err == nil {
		t.Fatal("expected an error from truncated/invalid JSON, got nil")
	}
}
