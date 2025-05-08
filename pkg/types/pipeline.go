package types

import (
	"fmt"
	"path/filepath"

	"github.com/Azure/ARO-Tools/pkg/config"
	"gopkg.in/yaml.v3"
)

type Pipeline struct {
	schema           string `yaml:"$schema,omitempty"`
	pipelineFilePath string
	ServiceGroup     string           `yaml:"serviceGroup"`
	RolloutName      string           `yaml:"rolloutName"`
	ResourceGroups   []*ResourceGroup `yaml:"resourceGroups"`
}

func NewPipelineFromFile(pipelineFilePath string, cfg config.Configuration) (*Pipeline, error) {
	bytes, err := config.PreprocessFile(pipelineFilePath, cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to preprocess pipeline file %w", err)
	}

	err = ValidatePipelineSchema(bytes)
	if err != nil {
		return nil, fmt.Errorf("failed to validate pipeline schema: %w", err)
	}

	absPath, err := filepath.Abs(pipelineFilePath)
	if err != nil {
		return nil, fmt.Errorf("failed to get absolute path for pipeline file %q: %w", pipelineFilePath, err)
	}

	pipeline, err := NewPlainPipelineFromBytes(absPath, bytes)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal pipeline file %w", err)
	}
	err = pipeline.Validate()
	if err != nil {
		return nil, fmt.Errorf("pipeline file failed validation %w", err)
	}
	return pipeline, nil
}
func NewPlainPipelineFromBytes(filepath string, bytes []byte) (*Pipeline, error) {
	rawPipeline := &struct {
		Schema         string `yaml:"$schema,omitempty"`
		ServiceGroup   string `yaml:"serviceGroup"`
		RolloutName    string `yaml:"rolloutName"`
		ResourceGroups []struct {
			Name         string           `yaml:"name"`
			Subscription string           `yaml:"subscription"`
			AKSCluster   string           `yaml:"aksCluster,omitempty"`
			Steps        []map[string]any `yaml:"steps"`
		} `yaml:"resourceGroups"`
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
		schema:           rawPipeline.Schema,
		pipelineFilePath: filepath,
		ServiceGroup:     rawPipeline.ServiceGroup,
		RolloutName:      rawPipeline.RolloutName,
		ResourceGroups:   make([]*ResourceGroup, len(rawPipeline.ResourceGroups)),
	}

	for i, rawRg := range rawPipeline.ResourceGroups {
		rg := &ResourceGroup{}
		pipeline.ResourceGroups[i] = rg
		rg.Name = rawRg.Name
		rg.Subscription = rawRg.Subscription
		rg.AKSCluster = rawRg.AKSCluster
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

func (p *Pipeline) DeepCopy(newPipelineFilePath string) (*Pipeline, error) {
	data, err := yaml.Marshal(p)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal pipeline: %v", err)
	}

	copy, err := NewPlainPipelineFromBytes(newPipelineFilePath, data)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal pipeline: %v", err)
	}
	return copy, nil
}

func (p *Pipeline) PipelineFilePath() string {
	return p.pipelineFilePath
}

func (p *Pipeline) AbsoluteFilePath(filePath string) (string, error) {
	return filepath.Abs(filepath.Join(filepath.Dir(p.pipelineFilePath), filePath))
}
