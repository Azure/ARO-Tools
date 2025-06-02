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
	"strings"
)

// Variable
// Use this to pass in values to pipeline steps. Values can come from various sources:
//   - Value: Use the value field to "hardcode" a value.
//   - ConfigRef: Use this to reference an entry in a config.Configuration.
//   - Input: Use this to specify an output chaining input.
type Variable struct {
	Name      string `yaml:"name,omitempty"`
	Value     any    `yaml:"value,omitempty"`
	ConfigRef string `yaml:"configRef,omitempty"`
	Input     *Input `yaml:"input,omitempty"`
}

// Input
// Holds the values used for output chaining:
//   - Step: Referenced step
type Input struct {
	Name string `yaml:"name"`
	Step string `yaml:"step"`
}

// Validate checks if the variable is valid
// Parameters:
//   - data: The configuration used to validate the Variable
//
// Returns:
//   - error: error in case there is any
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
