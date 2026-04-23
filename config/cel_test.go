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

package config

import (
	"encoding/json"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/stretchr/testify/require"
)

func compileCELSchema(schema string) (*jsonschema.Schema, error) {
	var schemaObj any
	if err := json.Unmarshal([]byte(schema), &schemaObj); err != nil {
		return nil, err
	}

	celVocab, err := NewCELVocabulary()
	if err != nil {
		return nil, err
	}
	c := jsonschema.NewCompiler()
	c.RegisterVocabulary(celVocab)
	c.AssertVocabs()
	if err := c.AddResource("schema.json", schemaObj); err != nil {
		return nil, err
	}
	return c.Compile("schema.json")
}

func compileWithCEL(t *testing.T, schema string) *jsonschema.Schema {
	t.Helper()
	sch, err := compileCELSchema(schema)
	require.NoError(t, err)
	return sch
}

func TestCELVocabulary(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		schema     string
		value      map[string]any
		wantErrors []string
	}{
		{
			name: "rule passes",
			schema: `{
				"type": "object",
				"properties": {
					"enabled": {"type": "boolean"},
					"name": {"type": "string"}
				},
				"x-cel-validations": [
					{"rule": "self.enabled == true", "message": "must be enabled"}
				]
			}`,
			value: map[string]any{
				"enabled": true,
				"name":    "test",
			},
		},
		{
			name: "rule fails with message",
			schema: `{
				"type": "object",
				"properties": {
					"enabled": {"type": "boolean"},
					"name": {"type": "string"}
				},
				"x-cel-validations": [
					{"rule": "self.enabled == true", "message": "must be enabled"}
				]
			}`,
			value: map[string]any{
				"enabled": false,
				"name":    "test",
			},
			wantErrors: []string{"must be enabled"},
		},
		{
			name: "multiple rules all evaluated",
			schema: `{
				"type": "object",
				"properties": {
					"port": {"type": "integer"},
					"name": {"type": "string"}
				},
				"x-cel-validations": [
					{"rule": "self.port > 0", "message": "port must be positive"},
					{"rule": "size(self.name) > 0", "message": "name must not be empty"}
				]
			}`,
			value: map[string]any{
				"port": int64(-1),
				"name": "",
			},
			wantErrors: []string{"port must be positive", "name must not be empty"},
		},
		{
			name: "nested rules at different depths",
			schema: `{
				"type": "object",
				"properties": {
					"database": {
						"type": "object",
						"properties": {
							"host": {"type": "string"},
							"port": {"type": "integer"}
						},
						"x-cel-validations": [
							{"rule": "self.port > 0 && self.port < 65536", "message": "port must be valid"}
						]
					}
				}
			}`,
			value: map[string]any{
				"database": map[string]any{
					"host": "localhost",
					"port": int64(99999),
				},
			},
			wantErrors: []string{"port must be valid"},
		},
		{
			name: "has() checks optional fields",
			schema: `{
				"type": "object",
				"properties": {
					"private": {"type": "boolean"},
					"endpoint": {"type": "string"}
				},
				"x-cel-validations": [
					{"rule": "!has(self.private) || !self.private || has(self.endpoint)", "message": "private resources must have an endpoint"}
				]
			}`,
			value: map[string]any{
				"private": true,
			},
			wantErrors: []string{"private resources must have an endpoint"},
		},
		{
			name: "has() passes when optional field absent",
			schema: `{
				"type": "object",
				"properties": {
					"private": {"type": "boolean"},
					"endpoint": {"type": "string"}
				},
				"x-cel-validations": [
					{"rule": "!has(self.private) || !self.private || has(self.endpoint)", "message": "private resources must have an endpoint"}
				]
			}`,
			value: map[string]any{
				"name": "test",
			},
		},
		{
			name: "cross-field string validation",
			schema: `{
				"type": "object",
				"properties": {
					"prefix": {"type": "string"},
					"fullName": {"type": "string"}
				},
				"x-cel-validations": [
					{"rule": "self.fullName.startsWith(self.prefix)", "message": "fullName must start with prefix"}
				]
			}`,
			value: map[string]any{
				"prefix":   "aro-",
				"fullName": "aro-hcp-cluster",
			},
		},
		{
			name: "cross-field string validation fails",
			schema: `{
				"type": "object",
				"properties": {
					"prefix": {"type": "string"},
					"fullName": {"type": "string"}
				},
				"x-cel-validations": [
					{"rule": "self.fullName.startsWith(self.prefix)", "message": "fullName must start with prefix"}
				]
			}`,
			value: map[string]any{
				"prefix":   "aro-",
				"fullName": "hcp-cluster",
			},
			wantErrors: []string{"fullName must start with prefix"},
		},
		{
			name: "empty x-cel-validations array is a no-op",
			schema: `{
				"type": "object",
				"properties": {
					"name": {"type": "string"}
				},
				"x-cel-validations": []
			}`,
			value: map[string]any{
				"name": "test",
			},
		},
		{
			name: "no x-cel-validations is a no-op",
			schema: `{
				"type": "object",
				"properties": {
					"name": {"type": "string"}
				}
			}`,
			value: map[string]any{
				"name": "test",
			},
		},
		{
			name: "non-object value skipped gracefully",
			schema: `{
				"type": "object",
				"properties": {
					"name": {"type": "string"}
				},
				"x-cel-validations": [
					{"rule": "size(self) > 0", "message": "must not be empty"}
				]
			}`,
			value: map[string]any{
				"name": "test",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			sch := compileWithCEL(t, tt.schema)
			err := sch.Validate(tt.value)
			if len(tt.wantErrors) == 0 {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
				for _, want := range tt.wantErrors {
					require.Contains(t, err.Error(), want)
				}
			}
		})
	}
}

func TestCELVocabulary_CompileErrors(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		schema     string
		wantErrors []string
	}{
		{
			name: "invalid CEL syntax",
			schema: `{
				"type": "object",
				"x-cel-validations": [
					{"rule": "self.foo ???", "message": "bad syntax"}
				]
			}`,
			wantErrors: []string{"failed to parse CEL expression"},
		},
		{
			name: "non-bool return type",
			schema: `{
				"type": "object",
				"x-cel-validations": [
					{"rule": "self.name", "message": "returns string not bool"}
				]
			}`,
			wantErrors: []string{"must return bool"},
		},
		{
			name: "missing rule field",
			schema: `{
				"type": "object",
				"x-cel-validations": [
					{"message": "no rule"}
				]
			}`,
			wantErrors: []string{"missing \"rule\" field"},
		},
		{
			name: "missing message field",
			schema: `{
				"type": "object",
				"x-cel-validations": [
					{"rule": "true"}
				]
			}`,
			wantErrors: []string{"missing \"message\" field"},
		},
		{
			name: "x-cel-validations is not an array",
			schema: `{
				"type": "object",
				"x-cel-validations": "not-an-array"
			}`,
			wantErrors: []string{"x-cel-validations must be an array"},
		},
		{
			name: "rule element is not an object",
			schema: `{
				"type": "object",
				"x-cel-validations": ["not-an-object"]
			}`,
			wantErrors: []string{"must be an object"},
		},
		{
			name: "empty rule field",
			schema: `{
				"type": "object",
				"x-cel-validations": [
					{"rule": "", "message": "msg"}
				]
			}`,
			wantErrors: []string{"missing \"rule\" field"},
		},
		{
			name: "empty message field",
			schema: `{
				"type": "object",
				"x-cel-validations": [
					{"rule": "true", "message": ""}
				]
			}`,
			wantErrors: []string{"missing \"message\" field"},
		},
		{
			name: "undeclared function in CEL expression",
			schema: `{
				"type": "object",
				"x-cel-validations": [
					{"rule": "unknownFunc(self)", "message": "should fail check"}
				]
			}`,
			wantErrors: []string{"failed to check CEL expression"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := compileCELSchema(tt.schema)
			require.Error(t, err)
			for _, want := range tt.wantErrors {
				require.Contains(t, err.Error(), want)
			}
		})
	}
}
