package types

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"
)

const (
	BicepExtension      = ".bicep"
	BicepParamExtension = ".bicepparam"
)

type ARMStep struct {
	// Required fields
	Template        string `yaml:"template"`
	Parameters      string `yaml:"parameters"`
	DeploymentLevel string `yaml:"deploymentLevel,omitempty"`
	DeploymentMode  string `yaml:"deploymentMode,omitempty"`

	// Optional fields
	Variables []Variable `yaml:"variables"`
}

// Validate validates the ARM template and parameters files and the deployment level.
func (a *ARMStep) Validate(data map[string]any) error {
	if filepath.Ext(a.Template) != BicepExtension {
		return fmt.Errorf("invalid template file extension %s", a.Template)
	}

	if filepath.Ext(a.Parameters) != BicepParamExtension {
		return fmt.Errorf("invalid parameters file extension %s", a.Parameters)
	}

	// The deployment level for the ARM template. Valid values are: "ResourceGroup", "Subscription", "ManagementGroup", "Tenant"
	deploymentLevels := []string{"resourcegroup", "subscription", "managementgroup", "tenant"}
	if !slices.Contains(deploymentLevels, strings.ToLower(a.DeploymentLevel)) {
		return fmt.Errorf("invalid deployment level %s, must be one of: %s", a.DeploymentLevel, strings.Join(deploymentLevels, ", "))
	}

	// 	The deployment mode for ARM template. Default value is incremental. Allowed options are incremental and complete.
	deploymentModes := []string{"", "incremental", "complete"}
	if !slices.Contains(deploymentModes, strings.ToLower(a.DeploymentMode)) {
		return fmt.Errorf("invalid deployment mode %s, must be one of: %s", a.DeploymentMode, strings.Join(deploymentModes, ", "))
	}

	for i := range a.Variables {
		if err := a.Variables[i].Validate(data); err != nil {
			return err
		}
	}

	return nil
}
