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

func TestBuildLogURLFromViewURL(t *testing.T) {
	tests := []struct {
		name    string
		viewURL string
		want    string
		wantErr bool
	}{
		{
			name:    "typical prow deck view URL",
			viewURL: "https://prow.ci.openshift.org/view/gs/origin-ci-test/logs/branch-ci-Azure-ARO-HCP-main-e2e-stage-e2e-parallel/12345",
			want:    "https://storage.googleapis.com/origin-ci-test/logs/branch-ci-Azure-ARO-HCP-main-e2e-stage-e2e-parallel/12345/build-log.txt",
		},
		{
			name:    "trailing slash is trimmed",
			viewURL: "https://prow.ci.openshift.org/view/gs/origin-ci-test/logs/some-job/1/",
			want:    "https://storage.googleapis.com/origin-ci-test/logs/some-job/1/build-log.txt",
		},
		{
			name:    "empty URL",
			viewURL: "",
			wantErr: true,
		},
		{
			name:    "missing /view/gs/ prefix",
			viewURL: "https://prow.ci.openshift.org/job-history/gs/origin-ci-test/logs/some-job",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := buildLogURLFromViewURL(tt.viewURL)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected an error, got url %q", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFetchLogContainsEV2RetryMarker(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		statusCode int
		want       bool
		wantErr    bool
	}{
		{
			name:       "marker present",
			body:       "some test output\nEV2_RETRY_ALLOWED: 1 known-issue test(s) failed (max 2 allowed), all labeled \"allow-retry\": [\"foo\"]\nmore output",
			statusCode: http.StatusOK,
			want:       true,
		},
		{
			name:       "marker absent",
			body:       "some test output\nsome other failure\nmore output",
			statusCode: http.StatusOK,
			want:       false,
		},
		{
			name:       "non-200 status",
			body:       "not found",
			statusCode: http.StatusNotFound,
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.statusCode)
				_, _ = w.Write([]byte(tt.body))
			}))
			defer server.Close()

			got, err := fetchLogContainsEV2RetryMarker(testContext(), server.URL)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected an error, got %v", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFetchLogContainsEV2RetryMarkerTruncatesLargeLogs(t *testing.T) {
	// The marker appears only past maxBuildLogBytes; the reader should stop
	// before reaching it and report false, not an error.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(strings.Repeat("x", maxBuildLogBytes+1024)))
		_, _ = w.Write([]byte(ev2RetryMarker))
	}))
	defer server.Close()

	got, err := fetchLogContainsEV2RetryMarker(testContext(), server.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got {
		t.Fatalf("expected marker beyond the read cap to be ignored, but it was found")
	}
}
