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
	query := kustoDiscoveryQuery()

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
