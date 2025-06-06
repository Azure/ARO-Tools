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
	"strings"
)

type RPStep struct {
	StepMeta `yaml:",inline"`

	// Required fields
	Namespaces Variable `yaml:"resourceProviderNamespaces"`
}

// NewRPStep creates a new RP Step
func NewRPStep(name string) *RPStep {
	return &RPStep{
		StepMeta: StepMeta{
			Name:   name,
			Action: "ResourceProviderRegistration",
		},
	}
}

// WithDependsOn fluent method that sets DependsOn
func (s *RPStep) WithDependsOn(dependsOn ...string) *RPStep {
	s.DependsOn = dependsOn
	return s
}

// WithNamespaces fluent method that sets parent
func (s *RPStep) WithNamespaces(namespaces Variable) *RPStep {
	s.Namespaces = namespaces
	return s
}

// Description
// Returns:
//   - A string representation of this RPStep
func (s *RPStep) Description() string {
	var details []string
	details = append(details, fmt.Sprintf("Namespace: %v", s.Namespaces))
	return fmt.Sprintf("Step %s\n  Kind: %s\n  %s", s.Name, s.Action, strings.Join(details, "\n  "))
}
