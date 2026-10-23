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

package config_test

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/Azure/ARO-Tools/config"
	"github.com/Azure/ARO-Tools/config/types"
)

func TestEV2Mapping(t *testing.T) {
	tests := []struct {
		name              string
		input             types.Configuration
		generator         config.PlaceholderGenerator
		prefix            []string
		expectedFlattened map[string]string
		expectedReplace   map[string]interface{}
	}{
		{
			name: "dunder",
			input: types.Configuration{
				"key1": "value1",
				"key2": 42,
				"key3": true,
				"parent": map[string]interface{}{
					"nested":    "nestedvalue",
					"nestedInt": 42,
					"deeper":    map[string]interface{}{"deepest": "deepestvalue"},
				},
			},
			generator: config.NewDunderPlaceholders(),
			expectedFlattened: map[string]string{
				"__key1__":                  "key1",
				"__key2__":                  "key2",
				"__key3__":                  "key3",
				"__parent.nested__":         "parent.nested",
				"__parent.nestedInt__":      "parent.nestedInt",
				"__parent.deeper.deepest__": "parent.deeper.deepest",
			},
			expectedReplace: map[string]interface{}{
				"key1": "__key1__",
				"key2": "__key2__",
				"key3": "__key3__",
				"parent": map[string]interface{}{
					"nested":    "__parent.nested__",
					"nestedInt": "__parent.nestedInt__",
					"deeper":    map[string]interface{}{"deepest": "__parent.deeper.deepest__"},
				},
			},
		},
		{
			name: "prefixed type-aware placeholders retain binding keys",
			input: types.Configuration{
				"name": "service",
				"settings": map[string]any{
					"replicas": 3,
					"regions":  []any{"eastus"},
					"optional": nil,
				},
			},
			generator: func(key []string, valueType reflect.Type) (string, string) {
				flattenedKey, replacement := config.NewDunderPlaceholders()(key, valueType)
				if valueType == nil || valueType.Kind() != reflect.String {
					replacement = fmt.Sprintf("any('%s')", replacement)
				}
				return flattenedKey, replacement
			},
			prefix: []string{"ev2"},
			expectedFlattened: map[string]string{
				"__ev2.name__":              "ev2.name",
				"__ev2.settings.replicas__": "ev2.settings.replicas",
				"__ev2.settings.regions__":  "ev2.settings.regions",
				"__ev2.settings.optional__": "ev2.settings.optional",
			},
			expectedReplace: map[string]interface{}{
				"name": "__ev2.name__",
				"settings": map[string]any{
					"replicas": "any('__ev2.settings.replicas__')",
					"regions":  "any('__ev2.settings.regions__')",
					"optional": "any('__ev2.settings.optional__')",
				},
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			flattened, replace := config.EV2Mapping(tc.input, tc.generator, tc.prefix)
			if diff := cmp.Diff(tc.expectedFlattened, flattened); diff != "" {
				t.Errorf("got incorrect flattened: %v", diff)
			}
			if diff := cmp.Diff(tc.expectedReplace, replace); diff != "" {
				t.Errorf("got incorrect replace: %v", diff)
			}
		})
	}
}
