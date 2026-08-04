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
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// ev2RetryMarker is the exact log line prefix that the ARO-HCP E2E test binary
// (aro-hcp-tests, see AROSLSRE-1721) prints when a run's failures are narrow
// enough to safely auto-retry: at most 2 tests failed, and every one of them
// was labeled allow-retry.
const ev2RetryMarker = "EV2_RETRY_ALLOWED:"

// maxBuildLogBytes caps how much of the build log we read looking for the
// retry marker, so a huge or slow log can't stall or blow up memory.
const maxBuildLogBytes = 4 << 20 // 4 MiB

// buildLogFetchTimeout bounds a single build-log fetch, independent of the
// overall job-monitoring timeout.
const buildLogFetchTimeout = 30 * time.Second

// buildLogURLFromViewURL converts a Prow Deck "view" URL (the one reported in
// ProwJob.Status.URL, e.g.
// https://prow.ci.openshift.org/view/gs/origin-ci-test/logs/<job>/<build>)
// into the plain-text build log's public GCS URL, e.g.
// https://storage.googleapis.com/origin-ci-test/logs/<job>/<build>/build-log.txt.
func buildLogURLFromViewURL(viewURL string) (string, error) {
	if viewURL == "" {
		return "", fmt.Errorf("job has no status URL, cannot locate its build log")
	}

	u, err := url.Parse(viewURL)
	if err != nil {
		return "", fmt.Errorf("failed to parse job status URL %q: %w", viewURL, err)
	}

	const viewPrefix = "/view/gs/"
	if !strings.HasPrefix(u.Path, viewPrefix) {
		return "", fmt.Errorf("job status URL %q does not look like a GCS Prow view URL (missing %q prefix)", viewURL, viewPrefix)
	}

	gcsPath := strings.Trim(strings.TrimPrefix(u.Path, viewPrefix), "/")
	return fmt.Sprintf("https://storage.googleapis.com/%s/build-log.txt", gcsPath), nil
}

// buildLogContainsEV2RetryMarker fetches the build log for the job reported
// at viewURL and reports whether it contains the EV2_RETRY_ALLOWED marker.
func buildLogContainsEV2RetryMarker(ctx context.Context, viewURL string) (bool, error) {
	rawLogURL, err := buildLogURLFromViewURL(viewURL)
	if err != nil {
		return false, err
	}
	return fetchLogContainsEV2RetryMarker(ctx, rawLogURL)
}

// fetchLogContainsEV2RetryMarker downloads rawLogURL and reports whether it
// contains the EV2_RETRY_ALLOWED marker. Split out from
// buildLogContainsEV2RetryMarker so the HTTP fetch/scan logic can be tested
// against a local httptest server, independent of GCS URL construction.
func fetchLogContainsEV2RetryMarker(ctx context.Context, rawLogURL string) (bool, error) {
	ctx, cancel := context.WithTimeout(ctx, buildLogFetchTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawLogURL, nil)
	if err != nil {
		return false, fmt.Errorf("failed to create build log request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false, fmt.Errorf("failed to fetch build log %q: %w", rawLogURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("failed to fetch build log %q: unexpected status %d", rawLogURL, resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBuildLogBytes))
	if err != nil {
		return false, fmt.Errorf("failed to read build log %q: %w", rawLogURL, err)
	}

	return strings.Contains(string(body), ev2RetryMarker), nil
}
