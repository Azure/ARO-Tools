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

package types

import (
	"strings"
	"testing"
)

func TestEv2OrchestratedStepName(t *testing.T) {
	for _, testCase := range []struct {
		name          string
		serviceGroup  string
		resourceGroup string
		step          string
		want          string
		wantError     string
	}{
		{
			name:          "valid",
			serviceGroup:  "Microsoft.Azure.ARO.HCP.Management.Infra",
			resourceGroup: "management",
			step:          "deploy",
			want:          "Management.Infra.management.deploy",
		},
		{
			name:          "exactly 70 bytes",
			serviceGroup:  "Microsoft.Azure.ARO.HCP." + strings.Repeat("s", 22),
			resourceGroup: strings.Repeat("r", 12),
			step:          strings.Repeat("t", 34),
			want:          strings.Repeat("s", 22) + "." + strings.Repeat("r", 12) + "." + strings.Repeat("t", 34),
		},
		{
			name:          "71 bytes with individually valid fields",
			serviceGroup:  "Microsoft.Azure.ARO.HCP." + strings.Repeat("s", 23),
			resourceGroup: strings.Repeat("r", 12),
			step:          strings.Repeat("t", 34),
			wantError:     "is 71 UTF-8 bytes; maximum is 70 bytes",
		},
		{
			name:          "reported rollout failure",
			serviceGroup:  "Microsoft.Azure.ARO.HCP.MSI.CredentialRefresherRBAC",
			resourceGroup: "management",
			step:          "remove-legacy-kv-refresher-stack",
			wantError:     `EV2 orchestrated step name "MSI.CredentialRefresherRBAC.management.remove-legacy-kv-refresher-stack" is 71 UTF-8 bytes`,
		},
		{
			name:          "reported rollout fix",
			serviceGroup:  "Microsoft.Azure.ARO.HCP.MSI.CredentialRefresherRBAC",
			resourceGroup: "management",
			step:          "remove-legacy-kv-refresh-stack",
			want:          "MSI.CredentialRefresherRBAC.management.remove-legacy-kv-refresh-stack",
		},
		{
			name:          "malformed service group",
			serviceGroup:  "Microsoft.Azure.ARO.HCP",
			resourceGroup: "global",
			step:          "deploy",
			wantError:     "expected at least 5 dot-separated parts",
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			got, err := Ev2OrchestratedStepName(testCase.serviceGroup, testCase.resourceGroup, testCase.step)
			if testCase.wantError != "" {
				if err == nil {
					t.Fatalf("expected an error containing %q, got %q", testCase.wantError, got)
				}
				if !strings.Contains(err.Error(), testCase.wantError) {
					t.Fatalf("error %q does not contain %q", err, testCase.wantError)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != testCase.want {
				t.Fatalf("got %q, want %q", got, testCase.want)
			}
		})
	}
}
