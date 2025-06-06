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

	"sigs.k8s.io/yaml"

	"github.com/Azure/ARO-Tools/pkg/config"
)

type Pipeline struct {
	Schema         string           `json:"$schema,omitempty"`
	ServiceGroup   string           `json:"serviceGroup"`
	RolloutName    string           `json:"rolloutName"`
	ResourceGroups []*ResourceGroup `json:"resourceGroups"`
}

// NewPipelineFromFile prepocesses and creates a new Pipeline instance from a file.
//
// Parameters:
//   - pipelineFilePath: The path to the pipeline file.
//   - cfg: The configuration object used for preprocessing the file.
//
// Returns:
//   - A pointer to a new Pipeline instance if successful.
//   - An error if there was a problem preprocessing the file, validating the schema,
//     unmarshaling the pipeline, or validating the pipeline instance.
func NewPipelineFromFile(pipelineFilePath string, cfg config.Configuration) (*Pipeline, error) {
	bytes, err := config.PreprocessFile(pipelineFilePath, cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to preprocess pipeline file: %w", err)
	}

	err = ValidatePipelineSchema(bytes)
	if err != nil {
		return nil, fmt.Errorf("failed to validate pipeline schema: %w", err)
	}

	pipeline, err := NewPlainPipelineFromBytes("", bytes)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal pipeline file: %w", err)
	}
	err = pipeline.Validate()
	if err != nil {
		return nil, fmt.Errorf("pipeline file failed validation: %w", err)
	}
	return pipeline, nil
}

// NewPlainPipelineFromBytes creates a new PlainPipeline instance from a YAML-encoded byte slice.
//
// Parameters:
//   - bytes: A byte slice containing YAML-encoded data representing a pipeline.
//
// Returns:
//   - A pointer to a new PlainPipeline instance, or an error if the input is invalid.
func NewPlainPipelineFromBytes(_ string, bytes []byte) (*Pipeline, error) {
	rawPipeline := &struct {
		Schema         string `json:"$schema,omitempty"`
		ServiceGroup   string `json:"serviceGroup"`
		RolloutName    string `json:"rolloutName"`
		ResourceGroups []struct {
			Name         string `json:"name"`
			Subscription string `json:"subscription"`
			// Deprecated: AKSCluster to be removed
			AKSCluster string           `json:"aksCluster,omitempty"`
			Steps      []map[string]any `json:"steps"`
		} `json:"resourceGroups"`
	}{}
	err := yaml.Unmarshal(bytes, rawPipeline)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal pipeline: %w", err)
	}

	// find step properties that are variableRefs
	pipelineSchema, _, err := getSchemaForRef(rawPipeline.Schema)
	if err != nil {
		return nil, fmt.Errorf("failed to get schema for pipeline: %w", err)
	}
	variableRefStepProperties, err := getVariableRefStepProperties(pipelineSchema)
	if err != nil {
		return nil, fmt.Errorf("failed to get variableRef step properties: %w", err)
	}

	pipeline := &Pipeline{
		Schema:         rawPipeline.Schema,
		ServiceGroup:   rawPipeline.ServiceGroup,
		RolloutName:    rawPipeline.RolloutName,
		ResourceGroups: make([]*ResourceGroup, len(rawPipeline.ResourceGroups)),
	}

	for i, rawRg := range rawPipeline.ResourceGroups {
		rg := &ResourceGroup{}
		pipeline.ResourceGroups[i] = rg
		rg.Name = rawRg.Name
		rg.Subscription = rawRg.Subscription
		rg.Steps = make([]Step, len(rawRg.Steps))
		for i, rawStep := range rawRg.Steps {
			// preprocess variableRef step properties
			for propName := range rawStep {
				if _, ok := variableRefStepProperties[propName]; ok {
					variableRef := rawStep[propName].(map[string]any)
					variableRef["name"] = propName
				}
			}

			// unmarshal the map into a StepMeta
			stepMeta := &StepMeta{}
			err := mapToStruct(rawStep, stepMeta)
			if err != nil {
				return nil, err
			}
			switch stepMeta.Action {
			case "Shell":
				rg.Steps[i] = &ShellStep{}
			case "ARM":
				rg.Steps[i] = &ARMStep{}
			default:
				rg.Steps[i] = &GenericStep{}
			}
			err = mapToStruct(rawStep, rg.Steps[i])
			if err != nil {
				return nil, err
			}
		}
	}

	// another round of validation after postprocessing
	err = ValidatePipelineSchemaForStruct(pipeline)
	if err != nil {
		return nil, fmt.Errorf("pipeline schema validation failed after postprocessing: %w", err)
	}

	return pipeline, nil
}

// Validate checks the integrity of the pipeline and its resource groups.
// It ensures that there are no duplicate step names, that all dependencies exist,
// and that each resource group is valid.
//
// Returns:
//   - An error if the pipeline or any of its resource groups are invalid.
//   - nil if the pipeline and all its resource groups are valid.
func (p *Pipeline) Validate() error {
	// collect all steps from all resourcegroups and fail if there are duplicates
	stepMap := make(map[string]Step)
	for _, rg := range p.ResourceGroups {
		for _, step := range rg.Steps {
			if _, ok := stepMap[step.StepName()]; ok {
				return fmt.Errorf("duplicate step name %q", step.StepName())
			}
			stepMap[step.StepName()] = step
		}
	}

	// validate dependsOn for a step exists
	for _, step := range stepMap {
		for _, dep := range step.Dependencies() {
			if _, ok := stepMap[dep]; !ok {
				return fmt.Errorf("invalid dependency on step %s: dependency %s does not exist", step.StepName(), dep)
			}
		}
	}

	// todo check for circular dependencies

	// validate resource groups
	for _, rg := range p.ResourceGroups {
		err := rg.Validate()
		if err != nil {
			return err
		}
	}
	return nil
}
