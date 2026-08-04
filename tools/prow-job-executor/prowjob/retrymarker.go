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
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/go-logr/logr"
)

// ev2RetryMarker is the exact log line prefix that the ARO-HCP E2E test binary
// (aro-hcp-tests, see AROSLSRE-1721) prints when a run's failures are narrow
// enough to safely auto-retry: at most 2 tests failed, and every one of them
// was labeled allow-retry.
const ev2RetryMarker = "EV2_RETRY_ALLOWED:"

// maxBuildLogBytes is how much of the tail of the build log we ask for. The
// marker is printed from an AfterAll, so it lands at the end of the log, and a
// full E2E log is far bigger than this.
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
	// aro-hcp-tests prints the marker from an AfterAll, so it is at the end of
	// the log. Ask for the tail, otherwise a long E2E log pushes the marker out
	// of anything we read from the front.
	req.Header.Set("Range", fmt.Sprintf("bytes=-%d", maxBuildLogBytes))

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false, fmt.Errorf("failed to fetch build log %q: %w", rawLogURL, err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			logr.FromContextOrDiscard(ctx).Error(err, "failed to close body")
		}
	}()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
		return false, fmt.Errorf("failed to fetch build log %q: unexpected status %d", rawLogURL, resp.StatusCode)
	}

	// 206 means we got just the tail. 200 means the server ignored the Range
	// header and is sending the whole log, so scan it as a stream rather than
	// buffering it. Either way the fetch timeout bounds how long this runs.
	found, err := streamContainsMarker(resp.Body)
	if err != nil {
		return false, fmt.Errorf("failed to read build log %q: %w", rawLogURL, err)
	}
	return found, nil
}

// streamContainsMarker reports whether r contains ev2RetryMarker, reading in
// fixed size chunks and carrying the last len(marker)-1 bytes over so a marker
// straddling a chunk boundary is still found. Memory stays constant regardless
// of how big the log is.
func streamContainsMarker(r io.Reader) (bool, error) {
	const chunkSize = 64 << 10
	marker := []byte(ev2RetryMarker)
	overlap := len(marker) - 1
	buf := make([]byte, overlap+chunkSize)
	filled := 0

	for {
		n, err := r.Read(buf[filled:])
		if n > 0 {
			filled += n
			if bytes.Contains(buf[:filled], marker) {
				return true, nil
			}
			if filled > overlap {
				copy(buf, buf[filled-overlap:filled])
				filled = overlap
			}
		}
		if err == io.EOF {
			return false, nil
		}
		if err != nil {
			return false, err
		}
	}
}
