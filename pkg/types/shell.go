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

import "fmt"

// ShellStep represents a shell step
// This struct supports fluent interface With... methods.
type ShellStep struct {
	StepMeta      `yaml:",inline"`
	AKSCluster    string      `yaml:"aksCluster,omitempty"`
	Command       string      `yaml:"command,omitempty"`
	Variables     []Variable  `yaml:"variables,omitempty"`
	DryRun        DryRun      `yaml:"dryRun,omitempty"`
	References    []Reference `yaml:"references,omitempty"`
	SubnetId      string      `yaml:"subnetId,omitempty"`
	ShellIdentity Variable    `yaml:"shellIdentity,omitempty"`
}

// Reference represents a configurable reference
type Reference struct {
	// Environment variable name
	Name string `yaml:"name"`

	// The path to a file.
	FilePath string `yaml:"filepath"`
}

// NewShellStep creates a new Shell step with the given parameters.
//
// Parameters:
//   - name: The name of the step.
//   - command: the command to execute.
//
// Returns:
//   - A pointer to an ShellStep struct, representing the newly created instance.
func NewShellStep(name string, command string) *ShellStep {
	return &ShellStep{
		StepMeta: StepMeta{
			Name:   name,
			Action: "Shell",
		},
		Command: command,
	}
}

// Description
// Returns:
//   - A string representation of this ShellStep
func (s *ShellStep) Description() string {
	return fmt.Sprintf("Step %s\n  Kind: %s\n  Command: %s\n", s.Name, s.Action, s.Command)
}

// WithAKSCluster fluent method that sets AKSCluster
func (s *ShellStep) WithAKSCluster(aksCluster string) *ShellStep {
	s.AKSCluster = aksCluster
	return s
}

// WithDependsOn fluent method that sets DependsOn
func (s *ShellStep) WithDependsOn(dependsOn ...string) *ShellStep {
	s.DependsOn = dependsOn
	return s
}

// WithVariables fluent method that sets Variables
func (s *ShellStep) WithVariables(variables ...Variable) *ShellStep {
	s.Variables = variables
	return s
}

// WithDryRun fluent method that sets DryRun
func (s *ShellStep) WithDryRun(dryRun DryRun) *ShellStep {
	s.DryRun = dryRun
	return s
}
