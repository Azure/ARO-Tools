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
	"time"
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

func TestFetchLogContainsEV2RetryMarkerFindsMarkerAtEndOfLargeLog(t *testing.T) {
	// The marker is printed from an AfterAll, so on a real E2E run it sits at
	// the very end of a log far larger than maxBuildLogBytes. Reading from the
	// front would miss it, which would silently disable the whole retry path.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log := strings.Repeat("x", maxBuildLogBytes+1024) + "\n" + ev2RetryMarker + " 1 known-issue test(s) failed\n"
		// http.ServeContent honours the Range header the same way GCS does.
		http.ServeContent(w, r, "build-log.txt", time.Time{}, strings.NewReader(log))
	}))
	defer server.Close()

	got, err := fetchLogContainsEV2RetryMarker(testContext(), server.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !got {
		t.Fatal("expected the marker at the end of a large log to be found, it was not")
	}
}

func TestFetchLogContainsEV2RetryMarkerScansWholeLogWhenRangeIgnored(t *testing.T) {
	// GCS honours Range, but if a server ignores it and returns the whole log
	// with 200 we still have to find a marker sitting past maxBuildLogBytes.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(strings.Repeat("x", maxBuildLogBytes+1024)))
		_, _ = w.Write([]byte("\n" + ev2RetryMarker + " 1 known-issue test(s) failed\n"))
	}))
	defer server.Close()

	got, err := fetchLogContainsEV2RetryMarker(testContext(), server.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !got {
		t.Fatal("expected the marker to be found when the server ignores Range, it was not")
	}
}

func TestStreamContainsMarkerAcrossChunkBoundary(t *testing.T) {
	// Split the marker so it straddles the internal chunk boundary, which is
	// what the carried-over overlap exists to handle.
	const chunkSize = 64 << 10
	for _, split := range []int{1, len(ev2RetryMarker) / 2, len(ev2RetryMarker) - 1} {
		padding := chunkSize - split
		body := strings.Repeat("x", padding) + ev2RetryMarker + "rest"
		got, err := streamContainsMarker(strings.NewReader(body))
		if err != nil {
			t.Fatalf("split %d: unexpected error: %v", split, err)
		}
		if !got {
			t.Fatalf("split %d: expected the marker straddling a chunk boundary to be found", split)
		}
	}

	got, err := streamContainsMarker(strings.NewReader(strings.Repeat("x", 3*chunkSize)))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got {
		t.Fatal("expected no marker in a log that does not contain one")
	}
}
