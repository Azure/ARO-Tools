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

type DNSStep struct {
	StepMeta `yaml:",inline"`

	// Required fields
	Parent Variable `yaml:"parentZone"`
	Child  Variable `yaml:"childZone"`
}

// NewDNSStep creates a new DNS Step
func NewDNSStep(name string) *DNSStep {
	return &DNSStep{
		StepMeta: StepMeta{
			Name:   name,
			Action: "DelegateChildZone",
		},
	}
}

// WithParent fluent method that sets parent
func (s *DNSStep) WithParent(parent Variable) *DNSStep {
	s.Parent = parent
	return s
}

// WithChild fluent method that sets parent
func (s *DNSStep) WithChild(child Variable) *DNSStep {
	s.Child = child
	return s
}

// Description
// Returns:
//   - A string representation of this ShellStep
func (s *DNSStep) Description() string {
	var details []string
	details = append(details, fmt.Sprintf("Parent: %v", s.Parent))
	details = append(details, fmt.Sprintf("Child: %v", s.Child))
	return fmt.Sprintf("Step %s\n  Kind: %s\n  %s", s.Name, s.Action, strings.Join(details, "\n  "))
}
