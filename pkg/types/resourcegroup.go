package types

import "fmt"

type ResourceGroup struct {
	Name         string `yaml:"name"`
	Subscription string `yaml:"subscription"`
	AKSCluster   string `yaml:"aksCluster,omitempty"`
	Steps        []Step `yaml:"steps"`
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
