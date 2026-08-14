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

func TestKustoDiscoveryQuery_NoEnvironment(t *testing.T) {
	query := kustoDiscoveryQuery("")

	if strings.Contains(query, "aroHCPEnvironment") {
		t.Fatalf("expected no environment filter when environment is empty, got: %s", query)
	}
	if !strings.Contains(query, "isnotempty(tags['aroHCPPurpose'])") {
		t.Fatalf("expected aroHCPPurpose filter, got: %s", query)
	}
	if !strings.Contains(query, "properties.provisioningState == 'Succeeded'") {
		t.Fatalf("expected provisioningState filter, got: %s", query)
	}
	if !strings.HasSuffix(query, "project name, location, uri=tostring(properties.uri), id") {
		t.Fatalf("expected projection to remain last, got: %s", query)
	}
}

func TestKustoDiscoveryQuery_WithEnvironment(t *testing.T) {
	query := kustoDiscoveryQuery("stg")

	if !strings.Contains(query, "tags['aroHCPEnvironment'] =~ 'stg'") {
		t.Fatalf("expected case-insensitive aroHCPEnvironment filter for stg, got: %s", query)
	}
	// The aroHCPPurpose filter must remain so discovery stays scoped to log clusters.
	if !strings.Contains(query, "isnotempty(tags['aroHCPPurpose'])") {
		t.Fatalf("expected aroHCPPurpose filter to remain, got: %s", query)
	}
	// The projection must remain the final clause for the row parser to work.
	if !strings.HasSuffix(query, "project name, location, uri=tostring(properties.uri), id") {
		t.Fatalf("expected projection to remain last, got: %s", query)
	}
}
