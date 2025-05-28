package types

import (
	"fmt"
	"strings"
)

// Variable
//  1. Hardcode: value is hardcoded
//     Name: variable name (eg: hello)
//     Value: hardcoded value  (eg: world)
//     Scope Bindings:
//     Find: __hello__
//     ReplaceWith: "world"
//  2. Configuration: configuration reference
//     ConfigRef: configuration key (eg: test)
//     Scope Bindings:
//     Find: __test__
//     ReplaceWith: "$config(test)"
//  3. Output chaining: value is outout from the previous step
//     Name: variable name (eg: test)
//     Input.Step: step name (eg: step)
//     Input.Name: output name (eg: output)
//     Scope Bindings:
//     Find: __test__
//     ReplaceWith:  "$serviceResourceDefinition(step).action(Deploy).outputs(output.value)"
type Variable struct {
	Name      string `yaml:"name,omitempty"`
	Value     any    `yaml:"value,omitempty"`
	ConfigRef string `yaml:"configRef,omitempty"`
	Input     *Input `yaml:"input,omitempty"`
}

type Input struct {
	Name string `yaml:"name"`
	Step string `yaml:"step"`
}

// Validate checks if the variable is valid
func (v *Variable) Validate(data map[string]any) error {
	if v.ConfigRef != "" {
		val, err := getConfigValue(data, v.ConfigRef)
		if err != nil {
			return err
		}

		v.Value = val
	} else if v.Value == nil && v.Input == nil {
		// In this case no value is set, clearly an error
		return fmt.Errorf("missing or empty value for variable %s", v.Name)
	}

	return nil
}

// getConfigValue returns the value for the key in the configuration
func getConfigValue(data map[string]any, key string) (any, error) {
	keys := strings.Split(key, ".")

	var result any = data

	for i, k := range keys {
		cast, matchesType := result.(map[string]any)
		if !matchesType {
			return nil, fmt.Errorf("%s: configuration value is %T, not %T", strings.Join(keys[0:i], "."), result, map[string]any{})
		}

		value, exists := cast[k]
		if !exists {
			return nil, fmt.Errorf("%s: configuration has no member %s", strings.Join(keys[0:i], "."), k)
		}

		result = value
	}

	return result, nil
}
