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
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/go-logr/logr"
)

// ev2RetryMetadataKey is the finished.json metadata key that the ARO-HCP E2E
// test binary (aro-hcp-tests, see AROSLSRE-1721) sets to true when a run's
// failures are narrow enough to safely auto-retry: at most 2 tests failed,
// and every one of them was labeled allow-retry. aro-hcp-tests writes this
// key into $ARTIFACT_DIR/metadata.json, which Prow's sidecar merges into the
// job's finished.json under the top-level "metadata" object - the standard
// Prow custom-metadata mechanism, so no log scraping is involved.
const ev2RetryMetadataKey = "ev2-retry-allowed"

// maxFinishedJSONBytes bounds how much of finished.json we'll read. The file
// is a small, flat JSON document; anything near this size indicates something
// unexpected and we'd rather fail closed than buffer an unbounded response.
const maxFinishedJSONBytes = 1 << 20 // 1 MiB

// finishedJSONFetchTimeout bounds a single finished.json fetch, independent of
// the overall job-monitoring timeout.
const finishedJSONFetchTimeout = 30 * time.Second

// finishedJSON is the subset of Prow's finished.json (produced by the sidecar
// utility, see sigs.k8s.io/prow/pkg/sidecar and the testgrid metadata.Finished
// type) that we need: the free-form metadata object merged in from each
// step's $ARTIFACT_DIR/metadata.json.
type finishedJSON struct {
	Metadata map[string]interface{} `json:"metadata"`
}

// finishedJSONURLFromViewURL converts a Prow Deck "view" URL (the one
// reported in ProwJob.Status.URL, e.g.
// https://prow.ci.openshift.org/view/gs/origin-ci-test/logs/<job>/<build>)
// into finished.json's public GCS URL, e.g.
// https://storage.googleapis.com/origin-ci-test/logs/<job>/<build>/finished.json.
func finishedJSONURLFromViewURL(viewURL string) (string, error) {
	if viewURL == "" {
		return "", fmt.Errorf("job has no status URL, cannot locate its finished.json")
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
	return fmt.Sprintf("https://storage.googleapis.com/%s/finished.json", gcsPath), nil
}

// jobAllowsEV2Retry fetches finished.json for the job reported at viewURL and
// reports whether its metadata carries ev2RetryMetadataKey=true.
func jobAllowsEV2Retry(ctx context.Context, viewURL string) (bool, error) {
	rawURL, err := finishedJSONURLFromViewURL(viewURL)
	if err != nil {
		return false, err
	}
	return fetchFinishedJSONAllowsRetry(ctx, rawURL)
}

// fetchFinishedJSONAllowsRetry downloads rawURL as a finished.json document
// and reports whether its metadata carries ev2RetryMetadataKey=true. Split out
// from jobAllowsEV2Retry so the HTTP fetch/parse logic can be tested against a
// local httptest server, independent of GCS URL construction.
func fetchFinishedJSONAllowsRetry(ctx context.Context, rawURL string) (bool, error) {
	ctx, cancel := context.WithTimeout(ctx, finishedJSONFetchTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return false, fmt.Errorf("failed to create finished.json request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false, fmt.Errorf("failed to fetch finished.json %q: %w", rawURL, err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			logr.FromContextOrDiscard(ctx).Error(err, "failed to close body")
		}
	}()

	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("failed to fetch finished.json %q: unexpected status %d", rawURL, resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxFinishedJSONBytes))
	if err != nil {
		return false, fmt.Errorf("failed to read finished.json %q: %w", rawURL, err)
	}

	var finished finishedJSON
	if err := json.Unmarshal(body, &finished); err != nil {
		return false, fmt.Errorf("failed to decode finished.json %q: %w", rawURL, err)
	}

	allow, _ := finished.Metadata[ev2RetryMetadataKey].(bool)
	return allow, nil
}
