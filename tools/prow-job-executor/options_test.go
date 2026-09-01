// Copyright 2025 Microsoft Corporation
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

package prowjobexecutor

import (
	"testing"

	"github.com/spf13/cobra"
)

// TestMonitorOptionsAbortOnCancelFlag verifies that the standalone "monitor"
// command exposes the same --abort-on-cancel flag as "execute" (defaulting to
// enabled) and that it can be disabled from the command line. Without this
// flag, monitoring an already-submitted job could never abort it on
// cancellation, unlike the "execute" path.
func TestMonitorOptionsAbortOnCancelFlag(t *testing.T) {
	opts := DefaultMonitorOptions()
	if !opts.AbortOnCancel {
		t.Fatalf("expected AbortOnCancel to default to true")
	}

	cmd := &cobra.Command{Use: "monitor"}
	if err := opts.BindFlags(cmd); err != nil {
		t.Fatalf("BindFlags returned error: %v", err)
	}

	if err := cmd.Flags().Set("execution-id", "00000000-0000-0000-0000-000000000000"); err != nil {
		t.Fatalf("failed to set execution-id: %v", err)
	}
	if err := cmd.Flags().Set("abort-on-cancel", "false"); err != nil {
		t.Fatalf("failed to set abort-on-cancel flag: %v", err)
	}

	if opts.AbortOnCancel {
		t.Fatalf("expected AbortOnCancel to be false after setting the flag")
	}
}

// TestRawMonitorOptionsValidateRejectsBadURLs verifies that Validate() catches
// a malformed --gangway-url/--prow-url before it ever reaches deriveBulkURL,
// rather than deriveBulkURL silently falling back to the raw, unparsed input.
func TestRawMonitorOptionsValidateRejectsBadURLs(t *testing.T) {
	tests := []struct {
		name       string
		gangwayURL string
		prowURL    string
		wantErr    bool
	}{
		{name: "valid URLs", gangwayURL: "https://gangway.example.com/v1/executions", prowURL: "https://prow.example.com/prowjob", wantErr: false},
		{name: "gangway-url missing scheme", gangwayURL: "gangway.example.com/v1/executions", prowURL: "https://prow.example.com/prowjob", wantErr: true},
		{name: "gangway-url not a URL at all", gangwayURL: "://not a url", prowURL: "https://prow.example.com/prowjob", wantErr: true},
		{name: "prow-url wrong scheme", gangwayURL: "https://gangway.example.com/v1/executions", prowURL: "ftp://prow.example.com/prowjob", wantErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			opts := DefaultMonitorOptions()
			opts.JobExecutionID = "00000000-0000-0000-0000-000000000000"
			opts.GangwayURL = tc.gangwayURL
			opts.ProwURL = tc.prowURL
			opts.KeyVaultURI = "https://vault.example.com"
			opts.Secret = "prow-token"

			_, err := opts.Validate(t.Context())
			if tc.wantErr && err == nil {
				t.Fatalf("expected an error, got nil")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("expected no error, got: %v", err)
			}
		})
	}
}
