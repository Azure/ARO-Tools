package types

import (
	"fmt"
	"strings"
)

type Variable struct {
	Name      string `yaml:"name"`
	Value     any    `yaml:"value"`
	ConfigRef string `yaml:"configRef"`
	Input     *Input `yaml:"input"`
}

type Input struct {
	Name string `yaml:"name"`
	Step string `yaml:"step"`
}

// Validate checks if the variable is valid
func (v *Variable) Validate(data map[string]any) error {
	if v.ConfigRef != "" {
		// Confugration
		val, err := getConfigValue(data, v.ConfigRef)
		if err != nil {
			return err
		}

		v.Value = val
	} else if v.Value == nil && v.Input == nil {
		// Hardcode
		// Output chaining
		return fmt.Errorf("missing or empty value for variable %s", v.Name)
	}

	return nil
}

// getConfigValue returns the value for the key in the configuration
func getConfigValue(data map[string]any, key string) (any, error) {
	err := fmt.Errorf("missing or empty value for key %s in configuration", key)
	keys := strings.Split(key, ".")

	var result any

	result = data

	for _, k := range keys {
		cast, matchesType := result.(map[string]any)
		if !matchesType {
			return nil, err
		}

		value, exists := cast[k]
		if !exists {
			return nil, err
		}

		result = value
	}

	return result, nil
}
