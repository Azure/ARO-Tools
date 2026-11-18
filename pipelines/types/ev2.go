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

// Ev2OrchestratedStepName composes and validates the name EV2 uses for a rollout step.
func Ev2OrchestratedStepName(serviceGroup, resourceGroupName, stepName string) (string, error) {
	parts := strings.Split(serviceGroup, ".")
	if len(parts) < 5 {
		return "", fmt.Errorf("invalid service group %q: expected at least 5 dot-separated parts", serviceGroup)
	}

	name := fmt.Sprintf("%s.%s.%s", strings.Join(parts[4:], "."), resourceGroupName, stepName)
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
