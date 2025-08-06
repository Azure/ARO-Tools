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
	"encoding/json"
	"fmt"

	"sigs.k8s.io/yaml"
)

// ResourceGroup represents a group of steps, targeting one resource group in one subscription.
type ResourceGroup struct {
	Name                     string                    `json:"name"`
	ResourceGroup            string                    `json:"resourceGroup"`
	Subscription             string                    `json:"subscription"`
	SubscriptionProvisioning *SubscriptionProvisioning `json:"subscriptionProvisioning,omitempty"`

	// ExecutionConstraints define a set of constraints on where this pipeline should be executed.
	// If unset, the default behavior is to deploy to all clouds, environments, regions, and stamps.
	// The set of constraints are evaluated using logical OR - adding to the list adds a set of possible
	// deployment environments.
	ExecutionConstraints []ExecutionConstraint `json:"executionConstraints,omitempty"`

	// Deprecated: AKSCluster to be removed
	AKSCluster string `json:"aksCluster,omitempty"`
	Steps      Steps  `json:"steps"`
}

// ExecutionConstraint defines a set of parameters for which the pipeline should execute. For each type of parameter,
// values are evaluated with logical OR - e.g. specifying two clouds will specify either cloud. The different types
// of parameters are evaluated with logical AND - e.g., specifying a cloud and an environment will constrain the
// pipeline to run in that cloud AND that environment.
type ExecutionConstraint struct {
	// Singleton defines this pipeline to run once - ever - in the entire history of the universe. No re-deployments
	// will be allowed.
	Singleton bool `json:"singleton"`

	// Clouds define the clouds in which this pipeline should run. If unset, execution will be unconstrained across clouds.
	Clouds []string `json:"clouds,omitempty"`
	// Environments define the environments in which this pipeline should run, for the given clouds. If unset, execution will be unconstrained across environments.
	Environments []string `json:"environments,omitempty"`
	// Regions define the regions in which this pipeline should run, for the given clouds and environments. If unset, execution will be unconstrained across regions.
	Regions []string `json:"regions,omitempty"`
}

func (rg *ResourceGroup) Validate() error {
	if rg.Name == "" {
		return fmt.Errorf("resource group name is required")
	}
	if rg.Subscription == "" {
		return fmt.Errorf("subscription is required")
	}
	return nil
}

type Steps []Step

func (s *Steps) UnmarshalJSON(data []byte) error {
	var raw []json.RawMessage
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("failed to unmarshal %v into array of json.RawMessage: %w", string(data), err)
	}

	steps := make([]Step, 0, len(raw))
	for i, rawStep := range raw {
		stepMeta := &StepMeta{}
		if err := yaml.Unmarshal(rawStep, stepMeta); err != nil {
			return fmt.Errorf("steps[%d]: failed to unmarshal step metadata from raw json: %w", i, err)
		}

		var step Step
		switch stepMeta.Action {
		case "Shell":
			step = &ShellStep{}
		case "ARM":
			step = &ARMStep{}
		case "DelegateChildZone":
			step = &DelegateChildZoneStep{}
		case "SetCertificateIssuer":
			step = &SetCertificateIssuerStep{}
		case "CreateCertificate":
			step = &CreateCertificateStep{}
		case "ResourceProviderRegistration":
			step = &ResourceProviderRegistrationStep{}
		case "ImageMirror":
			step = &ImageMirrorStep{}
		case "RPLogsAccount", "ClusterLogsAccount":
			step = &LogsStep{}
		case "FeatureRegistration":
			step = &FeatureRegistrationStep{}
		case "ProviderFeatureRegistration":
			step = &ProviderFeatureRegistrationStep{}
		case "Ev2Registration":
			step = &Ev2RegistrationStep{}
		case "SecretSync":
			step = &SecretSyncStep{}
		case "Kusto":
			step = &KustoStep{}
		case "Pav2ManageAppId":
			step = &Pav2ManageAppIdStep{}
		case "Pav2AddAccount":
			step = &Pav2AddAccountStep{}
		default:
			step = &GenericStep{}
		}
		if err := yaml.Unmarshal(rawStep, step); err != nil {
			return fmt.Errorf("steps[%d]: failed to unmarshal step from metadata remainder: %w", i, err)
		}
		steps = append(steps, step)
	}
	*s = steps
	return nil
}
