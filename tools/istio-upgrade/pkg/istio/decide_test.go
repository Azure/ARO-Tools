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

package istio

import (
	"testing"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
)

func TestDecide(t *testing.T) {
	tests := []struct {
		name       string
		state      UpgradeState
		target     string
		wantAction Action
	}{
		{
			name: "already at target",
			state: UpgradeState{
				ClusterName:            "svc-cluster-01",
				MeshProfileRevisions:   []string{"asm-1-28"},
				IstioAvailableUpgrades: []string{"asm-1-29"},
				ProvisioningState:      "Succeeded",
			},
			target:     "asm-1-28",
			wantAction: ActionReconcile,
		},
		{
			name: "upgrade available",
			state: UpgradeState{
				ClusterName:            "svc-cluster-01",
				MeshProfileRevisions:   []string{"asm-1-28"},
				IstioAvailableUpgrades: []string{"asm-1-29"},
				ProvisioningState:      "Succeeded",
			},
			target:     "asm-1-29",
			wantAction: ActionUpgrade,
		},
		{
			name: "mid-upgrade resume",
			state: UpgradeState{
				ClusterName:          "svc-cluster-01",
				MeshProfileRevisions: []string{"asm-1-28", "asm-1-29"},
				ProvisioningState:    "Succeeded",
			},
			target:     "asm-1-29",
			wantAction: ActionResume,
		},
		{
			name: "target not available in region",
			state: UpgradeState{
				ClusterName:            "svc-cluster-01",
				MeshProfileRevisions:   []string{"asm-1-27"},
				IstioAvailableUpgrades: []string{"asm-1-28"},
				KubernetesVersion:      "1.35",
				ProvisioningState:      "Succeeded",
			},
			target:     "asm-1-29",
			wantAction: ActionSkip,
		},
		{
			name: "downgrade detected",
			state: UpgradeState{
				ClusterName:          "svc-cluster-01",
				MeshProfileRevisions: []string{"asm-1-29"},
				ProvisioningState:    "Succeeded",
			},
			target:     "asm-1-28",
			wantAction: ActionSkip,
		},
		{
			name: "downgrade detected with multiple revisions",
			state: UpgradeState{
				ClusterName:          "svc-cluster-01",
				MeshProfileRevisions: []string{"asm-1-27", "asm-1-30"},
				ProvisioningState:    "Succeeded",
			},
			target:     "asm-1-29",
			wantAction: ActionSkip,
		},
		{
			name: "cluster in Failed state",
			state: UpgradeState{
				ClusterName:            "svc-cluster-01",
				MeshProfileRevisions:   []string{"asm-1-28"},
				IstioAvailableUpgrades: []string{"asm-1-29"},
				ProvisioningState:      "Failed",
			},
			target:     "asm-1-29",
			wantAction: ActionSkip,
		},
		{
			name: "new cluster install",
			state: UpgradeState{
				ClusterName:          "svc-cluster-01",
				MeshProfileRevisions: []string{},
				ProvisioningState:    "Succeeded",
			},
			target:     "asm-1-28",
			wantAction: ActionInstall,
		},
		{
			name: "new cluster nil revisions",
			state: UpgradeState{
				ClusterName:          "svc-cluster-01",
				MeshProfileRevisions: nil,
				ProvisioningState:    "Succeeded",
			},
			target:     "asm-1-28",
			wantAction: ActionInstall,
		},
		{
			name: "new cluster in Failed state skips install",
			state: UpgradeState{
				ClusterName:          "svc-cluster-01",
				MeshProfileRevisions: nil,
				ProvisioningState:    "Failed",
			},
			target:     "asm-1-28",
			wantAction: ActionSkip,
		},
		{
			name: "ARM default matches config - no action needed",
			state: UpgradeState{
				ClusterName:            "svc-cluster-01",
				MeshProfileRevisions:   []string{"asm-1-28"},
				IstioAvailableUpgrades: []string{"asm-1-29"},
				ProvisioningState:      "Succeeded",
			},
			target:     "asm-1-28",
			wantAction: ActionReconcile,
		},
		{
			name: "ARM default behind config - upgrade to reach target",
			state: UpgradeState{
				ClusterName:            "svc-cluster-01",
				MeshProfileRevisions:   []string{"asm-1-28"},
				IstioAvailableUpgrades: []string{"asm-1-29"},
				ProvisioningState:      "Succeeded",
			},
			target:     "asm-1-29",
			wantAction: ActionUpgrade,
		},
		{
			name: "ARM default ahead of config - downgrade skip",
			state: UpgradeState{
				ClusterName:            "svc-cluster-01",
				MeshProfileRevisions:   []string{"asm-1-29"},
				IstioAvailableUpgrades: []string{},
				ProvisioningState:      "Succeeded",
			},
			target:     "asm-1-28",
			wantAction: ActionSkip,
		},
		{
			name: "upgrade already in progress but target not yet installed skips",
			state: UpgradeState{
				ClusterName:            "svc-cluster-01",
				MeshProfileRevisions:   []string{"asm-1-28"},
				ProvisioningState:      "Succeeded",
				IstioUpgradeInProgress: true,
			},
			target:     "asm-1-29",
			wantAction: ActionSkip,
		},
		{
			name: "stale canary triggers cleanup and upgrade",
			state: UpgradeState{
				ClusterName:            "svc-cluster-01",
				MeshProfileRevisions:   []string{"asm-1-28", "asm-1-29"},
				IstioAvailableUpgrades: []string{"asm-1-30"},
				ProvisioningState:      "Succeeded",
			},
			target:     "asm-1-30",
			wantAction: ActionCleanupAndUpgrade,
		},
		{
			name: "stale canary skips when target unavailable",
			state: UpgradeState{
				ClusterName:            "svc-cluster-01",
				MeshProfileRevisions:   []string{"asm-1-28", "asm-1-29"},
				IstioAvailableUpgrades: []string{"asm-1-29"},
				ProvisioningState:      "Succeeded",
			},
			target:     "asm-1-30",
			wantAction: ActionSkip,
		},
		{
			name: "three or more revisions skips with manual intervention",
			state: UpgradeState{
				ClusterName:          "svc-cluster-01",
				MeshProfileRevisions: []string{"asm-1-27", "asm-1-28", "asm-1-29"},
				ProvisioningState:    "Succeeded",
			},
			target:     "asm-1-30",
			wantAction: ActionSkip,
		},
		{
			name: "single digit minor not treated as newer",
			state: UpgradeState{
				ClusterName:            "svc-cluster-01",
				MeshProfileRevisions:   []string{"asm-1-9"},
				IstioAvailableUpgrades: []string{"asm-1-28"},
				ProvisioningState:      "Succeeded",
			},
			target:     "asm-1-28",
			wantAction: ActionUpgrade,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			action := Decide(logr.Discard(), tt.state, tt.target)
			assert.Equal(t, tt.wantAction, action, "unexpected action for %q", tt.name)
		})
	}
}

func TestClassify(t *testing.T) {
	tests := []struct {
		name   string
		state  UpgradeState
		target string
		want   scenario
	}{
		{
			name:   "not ready",
			state:  UpgradeState{ProvisioningState: "Failed"},
			target: "asm-1-29",
			want:   scenarioNotReady,
		},
		{
			name:   "fresh install",
			state:  UpgradeState{ProvisioningState: "Succeeded"},
			target: "asm-1-29",
			want:   scenarioFreshInstall,
		},
		{
			name: "already at target",
			state: UpgradeState{
				ProvisioningState:    "Succeeded",
				MeshProfileRevisions: []string{"asm-1-29"},
			},
			target: "asm-1-29",
			want:   scenarioAlreadyAtTarget,
		},
		{
			name: "mid upgrade",
			state: UpgradeState{
				ProvisioningState:    "Succeeded",
				MeshProfileRevisions: []string{"asm-1-28", "asm-1-29"},
			},
			target: "asm-1-29",
			want:   scenarioMidUpgrade,
		},
		{
			name: "upgrade already in progress but target not yet installed",
			state: UpgradeState{
				ProvisioningState:      "Succeeded",
				MeshProfileRevisions:   []string{"asm-1-28"},
				IstioUpgradeInProgress: true,
			},
			target: "asm-1-29",
			want:   scenarioNotReady,
		},
		{
			name: "too many revisions",
			state: UpgradeState{
				ProvisioningState:    "Succeeded",
				MeshProfileRevisions: []string{"asm-1-27", "asm-1-28", "asm-1-29"},
			},
			target: "asm-1-30",
			want:   scenarioTooManyRevisions,
		},
		{
			name: "stale canary",
			state: UpgradeState{
				ProvisioningState:      "Succeeded",
				MeshProfileRevisions:   []string{"asm-1-28", "asm-1-29"},
				IstioAvailableUpgrades: []string{"asm-1-30"},
			},
			target: "asm-1-30",
			want:   scenarioStaleCanary,
		},
		{
			name: "stale canary with unavailable target",
			state: UpgradeState{
				ProvisioningState:      "Succeeded",
				MeshProfileRevisions:   []string{"asm-1-28", "asm-1-29"},
				IstioAvailableUpgrades: []string{"asm-1-29"},
			},
			target: "asm-1-30",
			want:   scenarioUpgradeUnavailable,
		},
		{
			name: "downgrade single revision",
			state: UpgradeState{
				ProvisioningState:    "Succeeded",
				MeshProfileRevisions: []string{"asm-1-29"},
			},
			target: "asm-1-28",
			want:   scenarioDowngrade,
		},
		{
			name: "downgrade multiple revisions",
			state: UpgradeState{
				ProvisioningState:    "Succeeded",
				MeshProfileRevisions: []string{"asm-1-27", "asm-1-30"},
			},
			target: "asm-1-29",
			want:   scenarioDowngrade,
		},
		{
			name: "upgrade available",
			state: UpgradeState{
				ProvisioningState:      "Succeeded",
				MeshProfileRevisions:   []string{"asm-1-28"},
				IstioAvailableUpgrades: []string{"asm-1-29"},
			},
			target: "asm-1-29",
			want:   scenarioUpgradeAvailable,
		},
		{
			name: "upgrade unavailable",
			state: UpgradeState{
				ProvisioningState:      "Succeeded",
				MeshProfileRevisions:   []string{"asm-1-28"},
				IstioAvailableUpgrades: []string{"asm-1-29"},
			},
			target: "asm-1-30",
			want:   scenarioUpgradeUnavailable,
		},
		{
			name: "upgrade unavailable with empty available list",
			state: UpgradeState{
				ProvisioningState:    "Succeeded",
				MeshProfileRevisions: []string{"asm-1-28"},
			},
			target: "asm-1-29",
			want:   scenarioUpgradeUnavailable,
		},
		{
			name: "3+ revisions with target present",
			state: UpgradeState{
				ProvisioningState:    "Succeeded",
				MeshProfileRevisions: []string{"asm-1-27", "asm-1-28", "asm-1-29"},
			},
			target: "asm-1-29",
			want:   scenarioMidUpgrade,
		},
		{
			name: "upgrade in progress with two revisions including target",
			state: UpgradeState{
				ProvisioningState:      "Succeeded",
				MeshProfileRevisions:   []string{"asm-1-28", "asm-1-29"},
				IstioUpgradeInProgress: true,
			},
			target: "asm-1-29",
			want:   scenarioMidUpgrade,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := classify(tt.state, tt.target)
			assert.Equal(t, tt.want, got, "unexpected scenario for %q", tt.name)
		})
	}
}

func TestCompareRevisions(t *testing.T) {
	tests := []struct {
		name string
		a, b string
		want int
	}{
		{"equal revisions", "asm-1-28", "asm-1-28", 0},
		{"a newer than b", "asm-1-29", "asm-1-28", 1},
		{"a older than b", "asm-1-28", "asm-1-29", -1},
		{"single digit vs double digit minor", "asm-1-9", "asm-1-28", -1},
		{"double digit vs single digit minor", "asm-1-28", "asm-1-9", 1},
		{"major version difference", "asm-2-1", "asm-1-99", 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := compareRevisions(tt.a, tt.b)
			if tt.want < 0 {
				assert.Negative(t, got, "compareRevisions(%q, %q)", tt.a, tt.b)
			} else if tt.want > 0 {
				assert.Positive(t, got, "compareRevisions(%q, %q)", tt.a, tt.b)
			} else {
				assert.Zero(t, got, "compareRevisions(%q, %q)", tt.a, tt.b)
			}
		})
	}
}
