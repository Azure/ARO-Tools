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

// ev2FailedTestsKey and ev2AllowRetryTestsKey are the finished.json metadata keys that the
// ARO-HCP E2E test binary (aro-hcp-tests, see AROSLSRE-1721) always writes once a run
// finishes: the full list of failed spec names, and the subset of those that were labeled
// allow-retry (a known, tracked issue with a fix already committed to). aro-hcp-tests
// writes both keys into $ARTIFACT_DIR/metadata.json, which Prow's sidecar merges into the
// job's finished.json under the top-level "metadata" object - the standard Prow
// custom-metadata mechanism, so no log scraping is involved.
//
// aro-hcp-tests only reports these raw facts, even when nothing failed (both lists empty);
// it is prow-job-executor's job (ev2RetryEligible below) to decide whether the shape of a
// failure is narrow enough to safely auto-retry. Keeping the policy here, rather than baked
// into an ARO-HCP release, means the retry threshold can be tuned without an ARO-HCP
// rebuild, and an absent key unambiguously means "this step never ran" rather than
// "the run failed but wasn't retry-eligible".
const (
	ev2FailedTestsKey     = "ev2-failed-tests"
	ev2AllowRetryTestsKey = "ev2-allow-retry-tests"
)

// DefaultMaxEV2AutoRetryFailures caps how many failed tests a run may have and still
// qualify for an automatic EV2 gating retry. If more tests than this fail, or any failure
// isn't labeled allow-retry, we stay silent and let the gate fail normally for a human to
// triage. Overridable via Monitor's maxAutoRetryFailures field (see the
// --max-ev2-auto-retry-failures flag).
const DefaultMaxEV2AutoRetryFailures = 2

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
	if gcsPath == "" {
		return "", fmt.Errorf("job status URL %q has an empty GCS path after the %q prefix", viewURL, viewPrefix)
	}
	return fmt.Sprintf("https://storage.googleapis.com/%s/finished.json", gcsPath), nil
}

// jobAllowsEV2Retry fetches finished.json for the job reported at viewURL and reports
// whether its ev2FailedTestsKey/ev2AllowRetryTestsKey metadata qualifies for an automatic
// EV2 gating retry, per ev2RetryEligible.
func jobAllowsEV2Retry(ctx context.Context, viewURL string, maxAutoRetryFailures int) (bool, error) {
	rawURL, err := finishedJSONURLFromViewURL(viewURL)
	if err != nil {
		return false, err
	}
	return fetchFinishedJSONAllowsRetry(ctx, rawURL, maxAutoRetryFailures)
}

// fetchFinishedJSONAllowsRetry downloads rawURL as a finished.json document and reports
// whether its metadata qualifies for an automatic EV2 gating retry. Split out from
// jobAllowsEV2Retry so the HTTP fetch/parse logic can be tested against a local httptest
// server, independent of GCS URL construction.
func fetchFinishedJSONAllowsRetry(ctx context.Context, rawURL string, maxAutoRetryFailures int) (bool, error) {
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

	// Read one byte past the cap so we can distinguish "fits within the cap" from
	// "was truncated" - io.LimitReader alone would silently accept an oversized body as
	// long as valid JSON appears before the limit, which defeats the fail-closed intent.
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxFinishedJSONBytes+1))
	if err != nil {
		return false, fmt.Errorf("failed to read finished.json %q: %w", rawURL, err)
	}
	if len(body) > maxFinishedJSONBytes {
		return false, fmt.Errorf("finished.json %q exceeds %d byte limit", rawURL, maxFinishedJSONBytes)
	}

	var finished finishedJSON
	if err := json.Unmarshal(body, &finished); err != nil {
		return false, fmt.Errorf("failed to decode finished.json %q: %w", rawURL, err)
	}

	failed, ok := stringSliceFromMetadata(finished.Metadata, ev2FailedTestsKey)
	if !ok {
		return false, fmt.Errorf("finished.json %q metadata key %q is present but not a list of strings", rawURL, ev2FailedTestsKey)
	}
	allowRetry, ok := stringSliceFromMetadata(finished.Metadata, ev2AllowRetryTestsKey)
	if !ok {
		return false, fmt.Errorf("finished.json %q metadata key %q is present but not a list of strings", rawURL, ev2AllowRetryTestsKey)
	}
	return ev2RetryEligible(failed, allowRetry, maxAutoRetryFailures), nil
}

// stringSliceFromMetadata extracts a []string out of finished.json's loosely-typed
// metadata map for the given key. A missing key returns (nil, true) - callers treat an
// absent list the same as an empty one. A key that's present but not a list of strings
// returns (nil, false) so the caller can fail closed instead of silently dropping the
// non-string elements and understating the failure count.
func stringSliceFromMetadata(metadata map[string]interface{}, key string) ([]string, bool) {
	rawVal, present := metadata[key]
	if !present {
		return nil, true
	}
	raw, ok := rawVal.([]interface{})
	if !ok {
		return nil, false
	}
	out := make([]string, 0, len(raw))
	for _, v := range raw {
		s, ok := v.(string)
		if !ok {
			return nil, false
		}
		out = append(out, s)
	}
	return out, true
}

// ev2RetryEligible decides whether a finished run qualifies for an automatic EV2 gating
// retry, given the raw facts aro-hcp-tests reported: every test name that failed, and the
// subset of those that were labeled allow-retry (a known, tracked issue). This is the
// whole eligibility policy, kept separate from the HTTP fetch/parse logic above so it can
// be tested directly.
//
// A run qualifies only when it failed at all, no more than maxAutoRetryFailures tests
// failed, and every failure was labeled allow-retry. Any other shape - too many failures,
// or even a single failure not carrying the label - disqualifies the whole run, so the
// gate fails normally and a human triages it.
func ev2RetryEligible(failed, allowRetry []string, maxAutoRetryFailures int) bool {
	if len(failed) == 0 || len(failed) > maxAutoRetryFailures {
		return false
	}
	allowRetrySet := make(map[string]bool, len(allowRetry))
	for _, name := range allowRetry {
		allowRetrySet[name] = true
	}
	for _, name := range failed {
		if !allowRetrySet[name] {
			return false
		}
	}
	return true
}
