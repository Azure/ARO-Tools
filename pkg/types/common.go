package types

import "fmt"

type StepMeta struct {
	Name      string   `yaml:"name"`
	Action    string   `yaml:"action"`
	DependsOn []string `yaml:"dependsOn,omitempty"`
}

func (m *StepMeta) StepName() string {
	return m.Name
}

func (m *StepMeta) ActionType() string {
	return m.Action
}

func (m *StepMeta) Dependencies() []string {
	return m.DependsOn
}

type Step interface {
	StepName() string
	ActionType() string
	Description() string
	Dependencies() []string
}

type GenericStep struct {
	StepMeta `yaml:",inline"`
	Body     map[string]any `yaml:",inline"`
}

func (s *GenericStep) Description() string {
	return fmt.Sprintf("Step %s\n  Kind: %s", s.Name, s.Action)
}

type DryRun struct {
	Variables []Variable `yaml:"variables,omitempty"`
	Command   string     `yaml:"command,omitempty"`
}

type Variable struct {
	Name      string `yaml:"name,omitempty"`
	ConfigRef string `yaml:"configRef,omitempty"`
	Value     string `yaml:"value,omitempty"`
	Input     *Input `yaml:"input,omitempty"`
}

type Input struct {
	Name string `yaml:"name"`
	Step string `yaml:"step"`
}
