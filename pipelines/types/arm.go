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
	"strings"
)

const StepActionARM = "ARM"

// ARMStep represents an ARM deployment step.
type ARMStep struct {
	StepMeta        `json:",inline"`
	Variables       []Variable `json:"variables,omitempty"`
	Template        string     `json:"template,omitempty"`
	Parameters      string     `json:"parameters,omitempty"`
	DeploymentLevel string     `json:"deploymentLevel,omitempty"`
	OutputOnly      bool       `json:"outputOnly,omitempty"`
	DeploymentMode  string     `json:"deploymentMode,omitempty"`
}

// Description
// Returns:
//   - A string representation of this ShellStep
func (s *ARMStep) Description() string {
	var details []string
	details = append(details, fmt.Sprintf("Template: %s", s.Template))
	details = append(details, fmt.Sprintf("Parameters: %s", s.Parameters))
	return fmt.Sprintf("Step %s\n  Kind: %s\n  %s", s.Name, s.Action, strings.Join(details, "\n  "))
}

func (s *ARMStep) RequiredInputs() []StepDependency {
	var deps []StepDependency
	for _, val := range s.Variables {
		if val.Input != nil {
			deps = append(deps, val.Input.StepDependency)
		}
	}
	slices.SortFunc(deps, SortDependencies)
	deps = slices.Compact(deps)
	return deps
}

const StepActionARMStack = "ARMStack"

// ARMStackStep represents an ARM stack deployment step.
type ARMStackStep struct {
	StepMeta        `json:",inline"`
	Variables       []Variable `json:"variables,omitempty"`
	Template        string     `json:"template,omitempty"`
	Parameters      string     `json:"parameters,omitempty"`
	DeploymentLevel string     `json:"deploymentLevel,omitempty"`

	// ActionOnUnmanage specifies what action to take when a resource previously owned by the deployment stack is no longer
	// specified. 'delete' will delete the resource, 'detach' will leave it as-is.
	ActionOnUnmanage string `json:"actionOnUnmanage,omitempty"`
	// BypassStackOutOfSyncError allows bypassing service errors that indicate the stack resource list is not correctly synchronized.
	BypassStackOutOfSyncError bool `json:"bypassStackOutOfSyncError,omitempty"`
}

// Description
// Returns:
//   - A string representation of this ShellStep
func (s *ARMStackStep) Description() string {
	var details []string
	details = append(details, fmt.Sprintf("Template: %s", s.Template))
	details = append(details, fmt.Sprintf("Parameters: %s", s.Parameters))
	return fmt.Sprintf("Step %s\n  Kind: %s\n  %s", s.Name, s.Action, strings.Join(details, "\n  "))
}

func (s *ARMStackStep) RequiredInputs() []StepDependency {
	var deps []StepDependency
	for _, val := range s.Variables {
		if val.Input != nil {
			deps = append(deps, val.Input.StepDependency)
		}
	}
	slices.SortFunc(deps, SortDependencies)
	deps = slices.Compact(deps)
	return deps
}

const StepActionARMStackDetach = "ARMStackDetach"

// ARMStackDetachStep removes an existing deployment stack and detaches every resource, resource group and
// management group it manages. It never deletes a managed resource, and it succeeds without making changes
// when the stack does not exist. Use it to release a stack left behind by an earlier ARMStack step, or to free
// resources before a differently named stack adopts them.
type ARMStackDetachStep struct {
	StepMeta        `json:",inline"`
	DeploymentLevel string `json:"deploymentLevel,omitempty"`

	// DeploymentStackName is the name of the existing deployment stack, exactly as Azure shows it. The stack is
	// looked up in the resource group this step runs in.
	DeploymentStackName string `json:"deploymentStackName"`

	// Timeout bounds the whole removal, as a Go duration string. The tool's default applies when unset.
	Timeout string `json:"timeout,omitempty"`

	// IdentityFrom specifies the managed identity with which this step will run in Ev2.
	IdentityFrom Input `json:"identityFrom,omitempty"`
}

// Description
// Returns:
//   - A string representation of this ARMStackDetachStep
func (s *ARMStackDetachStep) Description() string {
	var details []string
	details = append(details, fmt.Sprintf("DeploymentStackName: %s", s.DeploymentStackName))
	details = append(details, fmt.Sprintf("DeploymentLevel: %s", s.DeploymentLevel))
	return fmt.Sprintf("Step %s\n  Kind: %s\n  %s", s.Name, s.Action, strings.Join(details, "\n  "))
}

func (s *ARMStackDetachStep) RequiredInputs() []StepDependency {
	var deps []StepDependency
	if s.IdentityFrom.StepDependency != (StepDependency{}) {
		deps = append(deps, s.IdentityFrom.StepDependency)
	}
	return deps
}
