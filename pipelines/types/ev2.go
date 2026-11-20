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
	"fmt"
	"strings"
)

// MaxEv2OrchestratedStepNameBytes is the maximum UTF-8 byte length accepted by EV2.
const MaxEv2OrchestratedStepNameBytes = 70

// ev2ServiceGroupPrefixParts is the number of leading service-group segments,
// such as Microsoft.Azure.ARO.HCP, that EV2 omits from orchestrated step names.
const ev2ServiceGroupPrefixParts = 4

// Ev2OrchestratedStepName composes and validates the name EV2 uses for a rollout step.
func Ev2OrchestratedStepName(serviceGroup, resourceGroupName, stepName string) (string, error) {
	if !hasEv2ServiceGroupSuffix(serviceGroup) {
		return "", fmt.Errorf("invalid service group %q: expected at least %d dot-separated parts", serviceGroup, ev2ServiceGroupPrefixParts+1)
	}

	parts := strings.Split(serviceGroup, ".")
	name := fmt.Sprintf("%s.%s.%s", strings.Join(parts[ev2ServiceGroupPrefixParts:], "."), resourceGroupName, stepName)
	byteLength := len([]byte(name))
	if byteLength > MaxEv2OrchestratedStepNameBytes {
		return "", fmt.Errorf(
			"EV2 orchestrated step name %q is %d UTF-8 bytes; maximum is %d bytes",
			name,
			byteLength,
			MaxEv2OrchestratedStepNameBytes,
		)
	}

	return name, nil
}

// hasEv2ServiceGroupSuffix reports whether serviceGroup has segments after the
// EV2 prefix and therefore produces orchestrated step names.
func hasEv2ServiceGroupSuffix(serviceGroup string) bool {
	return len(strings.Split(serviceGroup, ".")) > ev2ServiceGroupPrefixParts
}
