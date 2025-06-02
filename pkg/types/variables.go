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
