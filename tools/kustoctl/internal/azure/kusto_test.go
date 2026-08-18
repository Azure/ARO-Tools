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

package azure

import (
	"strings"
	"testing"
)

func TestKustoDiscoveryQuery(t *testing.T) {
	query, err := kustoDiscoveryQuery(KustoDiscoveryConfig{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(query, "isnotempty(tags['aroHCPPurpose'])") {
		t.Fatalf("expected aroHCPPurpose filter, got: %s", query)
	}
	if !strings.Contains(query, "properties.provisioningState == 'Succeeded'") {
		t.Fatalf("expected provisioningState filter, got: %s", query)
	}
	// The aroHCPEnvironment tag is projected so the caller can scope membership
	// client-side; it must never be interpolated into a where clause, so there
	// is no KQL-injection surface from the environment value.
	if !strings.Contains(query, "environment=tostring(tags['aroHCPEnvironment'])") {
		t.Fatalf("expected aroHCPEnvironment to be projected, got: %s", query)
	}
	// aroHCPEnvironment must appear exactly once, in the projection above. A
	// second occurrence would mean it is referenced in a where/filter clause
	// (regardless of the operator, e.g. ==, =~, in~, has), which is the
	// KQL-injection surface this test guards against.
	if got := strings.Count(query, "aroHCPEnvironment"); got != 1 {
		t.Fatalf("expected aroHCPEnvironment to appear exactly once (in the projection), got %d occurrences in: %s", got, query)
	}
	// The projection must remain the final clause for the row parser to work.
	if !strings.HasSuffix(query, "environment=tostring(tags['aroHCPEnvironment'])") {
		t.Fatalf("expected projection to remain last, got: %s", query)
	}
}

func TestKustoDiscoveryQueryCustomTags(t *testing.T) {
	query, err := kustoDiscoveryQuery(KustoDiscoveryConfig{
		TagKey:      "classicPurpose",
		TagValue:    "logs",
		ScopeTagKey: "classicEnv",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// A non-empty purpose value produces an equality filter rather than a
	// presence check, so other products can target a specific tag value.
	if !strings.Contains(query, "tags['classicPurpose'] =~ 'logs'") {
		t.Fatalf("expected purpose value match, got: %s", query)
	}
	// The custom scope tag is projected, and (as with the default) must appear
	// exactly once so there is no where-clause interpolation of a scope value.
	if !strings.HasSuffix(query, "environment=tostring(tags['classicEnv'])") {
		t.Fatalf("expected custom scope tag projected last, got: %s", query)
	}
	if got := strings.Count(query, "classicEnv"); got != 1 {
		t.Fatalf("expected scope tag to appear exactly once, got %d in: %s", got, query)
	}
	// The ARO-HCP defaults must not leak in when custom tags are configured.
	if strings.Contains(query, "aroHCPEnvironment") || strings.Contains(query, "aroHCPPurpose") {
		t.Fatalf("did not expect default tags, got: %s", query)
	}
}

func TestKustoDiscoveryQueryRejectsInjection(t *testing.T) {
	cases := []struct {
		name string
		cfg  KustoDiscoveryConfig
	}{
		{"tag key with quote", KustoDiscoveryConfig{TagKey: "x'] or '1'=='1"}},
		{"scope key with bracket", KustoDiscoveryConfig{ScopeTagKey: "aroHCPEnvironment'])] //"}},
		{"tag value with quote", KustoDiscoveryConfig{TagValue: "logs' or '1'=='1"}},
		{"tag key with space", KustoDiscoveryConfig{TagKey: "aro HCP"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := kustoDiscoveryQuery(tc.cfg); err == nil {
				t.Fatalf("expected error for %s", tc.name)
			}
		})
	}
}
