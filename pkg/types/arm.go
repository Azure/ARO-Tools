package types

import (
	"fmt"
	"strings"
)

type ARMStep struct {
	StepMeta        `yaml:",inline"`
	Command         string     `yaml:"command,omitempty"`
	Variables       []Variable `yaml:"variables,omitempty"`
	Template        string     `yaml:"template,omitempty"`
	Parameters      string     `yaml:"parameters,omitempty"`
	DeploymentLevel string     `yaml:"deploymentLevel,omitempty"`
	OutputOnly      bool       `yaml:"outputOnly,omitempty"`
	DeploymentMode  string     `yaml:"deploymentMode,omitempty"`
}

func NewARMStep(name string, template string, parameters string, deploymentLevel string) *ARMStep {
	return &ARMStep{
		StepMeta: StepMeta{
			Name:   name,
			Action: "ARM",
		},
		Template:        template,
		Parameters:      parameters,
		DeploymentLevel: deploymentLevel,
	}
}

func (s *ARMStep) WithDependsOn(dependsOn ...string) *ARMStep {
	s.DependsOn = dependsOn
	return s
}

func (s *ARMStep) WithVariables(variables ...Variable) *ARMStep {
	s.Variables = variables
	return s
}

func (s *ARMStep) WithOutputOnly() *ARMStep {
	s.OutputOnly = true
	return s
}

func (s *ARMStep) Description() string {
	var details []string
	details = append(details, fmt.Sprintf("Template: %s", s.Template))
	details = append(details, fmt.Sprintf("Parameters: %s", s.Parameters))
	return fmt.Sprintf("Step %s\n  Kind: %s\n  %s", s.Name, s.Action, strings.Join(details, "\n  "))
}
