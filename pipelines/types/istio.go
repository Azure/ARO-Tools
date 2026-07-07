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

import "fmt"

const StepActionIstioUpgrade = "IstioUpgrade"

type IstioUpgradeStep struct {
	StepMeta   `json:",inline"`
	AKSCluster string `json:"aksCluster"`
	DryRun     bool   `json:"dryRun,omitempty"`
	// "install" installs the new control plane alongside the existing one
	// and stops before workload migration. "upgrade" migrates workloads and
	// completes the canary. Empty runs the full lifecycle in one step.
	Phase string `json:"phase,omitempty"`
}

func (s *IstioUpgradeStep) Description() string {
	phase := s.Phase
	if phase == "" {
		phase = "full"
	}
	return fmt.Sprintf("Step %s\n  Kind: %s\n  AKSCluster: %s\n  DryRun: %v\n  Phase: %s\n", s.Name, s.Action, s.AKSCluster, s.DryRun, phase)
}

func (s *IstioUpgradeStep) RequiredInputs() []StepDependency {
	return []StepDependency{}
}

func (s *IstioUpgradeStep) IsWellFormedOverInputs() bool {
	return true
}
