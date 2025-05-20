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

type ShellStep struct {
	StepMeta   `yaml:",inline"`
	AKSCluster string     `yaml:"aksCluster,omitempty"`
	Command    string     `yaml:"command,omitempty"`
	Variables  []Variable `yaml:"variables,omitempty"`
	DryRun     DryRun     `yaml:"dryRun,omitempty"`
}

func NewShellStep(name string, command string) *ShellStep {
	return &ShellStep{
		StepMeta: StepMeta{
			Name:   name,
			Action: "Shell",
		},
		Command: command,
	}
}

func (s *ShellStep) Description() string {
	return fmt.Sprintf("Step %s\n  Kind: %s\n  Command: %s\n", s.Name, s.Action, s.Command)
}

func (s *ShellStep) WithAKSCluster(aksCluster string) *ShellStep {
	s.AKSCluster = aksCluster
	return s
}

func (s *ShellStep) WithDependsOn(dependsOn ...string) *ShellStep {
	s.DependsOn = dependsOn
	return s
}

func (s *ShellStep) WithVariables(variables ...Variable) *ShellStep {
	s.Variables = variables
	return s
}

func (s *ShellStep) WithDryRun(dryRun DryRun) *ShellStep {
	s.DryRun = dryRun
	return s
}
