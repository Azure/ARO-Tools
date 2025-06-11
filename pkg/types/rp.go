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

package types

import (
	"fmt"
)

type ResourceProviderRegistrationStep struct {
	StepMeta `json:",inline"`

	// Required fields
	Namespaces Variable `json:"resourceProviderNamespaces"`
}

// NewRPStep creates a new RP Step
func NewRPStep(name string) *ResourceProviderRegistrationStep {
	return &ResourceProviderRegistrationStep{
		StepMeta: StepMeta{
			Name:   name,
			Action: "ResourceProviderRegistration",
		},
	}
}

// WithDependsOn fluent method that sets DependsOn
func (s *ResourceProviderRegistrationStep) WithDependsOn(dependsOn ...string) *ResourceProviderRegistrationStep {
	s.DependsOn = dependsOn
	return s
}

// WithNamespaces fluent method that sets parent
func (s *ResourceProviderRegistrationStep) WithNamespaces(namespaces Variable) *ResourceProviderRegistrationStep {
	s.Namespaces = namespaces
	return s
}

// Description
// Returns:
//   - A string representation of this ResourceProviderRegistrationStep
func (s *ResourceProviderRegistrationStep) Description() string {
	return fmt.Sprintf("Step %s\n Kind: %s\n", s.Name, s.Action)
}
