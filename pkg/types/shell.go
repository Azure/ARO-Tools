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
	"slices"
)

const StepActionShell = "Shell"

// Common base for ShellStep and ShellValidationStep
type ShellStepBase struct {
	AKSCluster    string      `json:"aksCluster,omitempty"`
	Command       string      `json:"command,omitempty"`
	Variables     []Variable  `json:"variables,omitempty"`
	DryRun        DryRun      `json:"dryRun,omitempty"`
	References    []Reference `json:"references,omitempty"`
	SubnetName    string      `json:"subnetName,omitempty"`
	ShellIdentity Value       `json:"shellIdentity,omitempty"`
}

// ShellStep represents a shell step
type ShellStep struct {
	StepMeta      `json:",inline"`
	ShellStepBase `json:",inline"`
}

// Reference represents a configurable reference
type Reference struct {
	// Environment variable name
	Name string `json:"name"`

	// The path to a file.
	FilePath string `json:"filepath"`
}

// Description
// Returns:
//   - A string representation of this ShellStep
func (s *ShellStep) Description() string {
	return fmt.Sprintf("Step %s\n  Kind: %s\n  Command: %s\n", s.Name, s.Action, s.Command)
}

func (s *ShellStepBase) RequiredInputs() []StepDependency {
	var deps []StepDependency
	for _, val := range append(s.Variables, s.DryRun.Variables...) {
		if val.Input != nil {
			deps = append(deps, val.Input.StepDependency)
		}
	}
	if s.ShellIdentity.Input != nil {
		deps = append(deps, s.ShellIdentity.Input.StepDependency)
	}
	slices.SortFunc(deps, SortDependencies)
	deps = slices.Compact(deps)
	return deps
}

// ShellValidationStep represents a shell step that is a validation step.
type ShellValidationStep struct {
	ValidationStepMeta `json:",inline"`
	ShellStepBase      `json:",inline"`
}

// Description
// Returns:
//   - A string representation of this ShellStep
func (s *ShellValidationStep) Description() string {
	return fmt.Sprintf("Validation Step %s\n  Kind: %s\n  Command: %s\n", s.Name, s.Action, s.Command)
}


func (s *ShellValidationStep) Validations() []string {
	return s.Validation
}
