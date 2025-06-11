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

type DelegateChildZoneStep struct {
	StepMeta `json:",inline"`

	// Required fields
	Parent Variable `json:"parentZone"`
	Child  Variable `json:"childZone"`
}

// NewDNSStep creates a new DNS Step
func NewDelegateChildZoneStep(name string) *DelegateChildZoneStep {
	return &DelegateChildZoneStep{
		StepMeta: StepMeta{
			Name:   name,
			Action: "DelegateChildZone",
		},
	}
}

// WithDependsOn fluent method that sets DependsOn
func (s *DelegateChildZoneStep) WithDependsOn(dependsOn ...string) *DelegateChildZoneStep {
	s.DependsOn = dependsOn
	return s
}

// WithParent fluent method that sets parent
func (s *DelegateChildZoneStep) WithParent(parent Variable) *DelegateChildZoneStep {
	s.Parent = parent
	return s
}

// WithChild fluent method that sets parent
func (s *DelegateChildZoneStep) WithChild(child Variable) *DelegateChildZoneStep {
	s.Child = child
	return s
}

// Description
// Returns:
//   - A string representation of this DelegateChildZoneStep
func (s *DelegateChildZoneStep) Description() string {
	return fmt.Sprintf("Step %s\n Kind: %s\n", s.Name, s.Action)
}
