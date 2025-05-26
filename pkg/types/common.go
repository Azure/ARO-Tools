package types

import "fmt"

type Step interface {
	StepName() string
	ActionType() string
	Description() string
	Dependencies() []string
}

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
