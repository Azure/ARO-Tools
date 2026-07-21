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
	"slices"
	"strconv"
	"strings"

	"github.com/go-logr/logr"
)

type Action string

const (
	ActionInstall           Action = "install"
	ActionUpgrade           Action = "upgrade"
	ActionSkip              Action = "skip"
	ActionResume            Action = "resume"
	ActionCleanupAndUpgrade Action = "cleanup-and-upgrade"
)

// UpgradeState combines cluster provisioning state with Istio mesh profile
// state to drive the upgrade decision engine.
type UpgradeState struct {
	ClusterName            string
	MeshProfileRevisions   []string
	IstioAvailableUpgrades []string
	KubernetesVersion      string
	ProvisioningState      string
	IstioUpgradeInProgress bool
}

type scenario int

const (
	scenarioNotReady          scenario = iota
	scenarioFreshInstall
	scenarioAlreadyAtTarget
	scenarioMidUpgrade
	scenarioTooManyRevisions
	scenarioStaleCanary
	scenarioDowngrade
	scenarioUpgradeAvailable
	scenarioUpgradeUnavailable
)

func classify(state UpgradeState, target string) scenario {
	if state.ProvisioningState != "Succeeded" {
		return scenarioNotReady
	}

	if state.IstioUpgradeInProgress {
		if slices.Contains(state.MeshProfileRevisions, target) {
			return scenarioMidUpgrade
		}
		return scenarioNotReady
	}

	if len(state.MeshProfileRevisions) == 0 {
		return scenarioFreshInstall
	}

	hasTarget := false
	hasOther := false
	for _, rev := range state.MeshProfileRevisions {
		if rev == target {
			hasTarget = true
		} else {
			hasOther = true
		}
	}

	if hasTarget && !hasOther {
		return scenarioAlreadyAtTarget
	}
	if hasTarget && hasOther {
		return scenarioMidUpgrade
	}

	if len(state.MeshProfileRevisions) > 2 {
		return scenarioTooManyRevisions
	}

	highest := slices.MaxFunc(state.MeshProfileRevisions, compareRevisions)
	if compareRevisions(highest, target) > 0 {
		return scenarioDowngrade
	}

	if !slices.Contains(state.IstioAvailableUpgrades, target) {
		return scenarioUpgradeUnavailable
	}

	// Single revision with target available — normal upgrade path.
	if len(state.MeshProfileRevisions) == 1 {
		return scenarioUpgradeAvailable
	}

	// Two revisions with target available — stale canary from a prior failed upgrade.
	return scenarioStaleCanary
}

func Decide(logger logr.Logger, state UpgradeState, target string) Action {
	sc := classify(state, target)
	switch sc {
	case scenarioNotReady:
		if state.IstioUpgradeInProgress {
			logger.Info("Skipping: ARM upgrade still provisioning, will retry",
				"provisioningState", state.ProvisioningState,
				"installed", state.MeshProfileRevisions)
		} else {
			logger.Info("Skipping: cluster not ready",
				"provisioningState", state.ProvisioningState)
		}
		return ActionSkip
	case scenarioFreshInstall:
		logger.Info("No revisions installed, installing from svc.istio.versions",
			"target", target)
		return ActionInstall
	case scenarioAlreadyAtTarget:
		logger.Info("Already at svc.istio.versions target",
			"target", target)
		return ActionSkip
	case scenarioMidUpgrade:
		logger.Info("Mid-upgrade detected, resuming",
			"installed", state.MeshProfileRevisions,
			"target", target)
		return ActionResume
	case scenarioTooManyRevisions:
		logger.Info("Unexpected revision count, manual intervention required",
			"revisions", state.MeshProfileRevisions,
			"count", len(state.MeshProfileRevisions))
		return ActionSkip
	case scenarioStaleCanary:
		logger.Info("Stale canary detected, will clean up and upgrade",
			"installed", state.MeshProfileRevisions,
			"target", target)
		return ActionCleanupAndUpgrade
	case scenarioDowngrade:
		highest := slices.MaxFunc(state.MeshProfileRevisions, compareRevisions)
		logger.Info("Downgrade detected, skipping",
			"installed", highest,
			"target", target)
		return ActionSkip
	case scenarioUpgradeAvailable:
		highest := slices.MaxFunc(state.MeshProfileRevisions, compareRevisions)
		logger.Info("Upgrading to svc.istio.versions target",
			"from", highest,
			"to", target,
			"k8sVersion", state.KubernetesVersion)
		return ActionUpgrade
	case scenarioUpgradeUnavailable:
		logger.Info("Target not in available upgrades, skipping",
			"target", target,
			"k8sVersion", state.KubernetesVersion)
		return ActionSkip
	default:
		logger.Info("Unhandled scenario, skipping")
		return ActionSkip
	}
}

func compareRevisions(a, b string) int {
	aParts := strings.Split(a, "-")
	bParts := strings.Split(b, "-")

	maxLen := max(len(aParts), len(bParts))

	for i := range maxLen {
		var aVal, bVal string
		if i < len(aParts) {
			aVal = aParts[i]
		}
		if i < len(bParts) {
			bVal = bParts[i]
		}

		aNum, aErr := strconv.Atoi(aVal)
		bNum, bErr := strconv.Atoi(bVal)

		if aErr == nil && bErr == nil {
			if aNum < bNum {
				return -1
			}
			if aNum > bNum {
				return 1
			}
			continue
		}

		if aVal < bVal {
			return -1
		}
		if aVal > bVal {
			return 1
		}
	}
	return 0
}
