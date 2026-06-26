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
	"reflect"
	"regexp"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/Azure/ARO-Tools/config/types"
)

// TestNewDunderPlaceholders pins the dunder generator's output format. The
// flattened-key and replace-variable are both "__<dotted.path>__"; the leaf
// reflect.Type is ignored.
func TestNewDunderPlaceholders(t *testing.T) {
	cases := []struct {
		name      string
		key       []string
		valueType reflect.Type
		want      string
	}{
		{
			name:      "single segment",
			key:       []string{"foo"},
			valueType: nil,
			want:      "__foo__",
		},
		{
			name:      "two segments",
			key:       []string{"foo", "bar"},
			valueType: nil,
			want:      "__foo.bar__",
		},
		{
			name:      "deep path",
			key:       []string{"a", "b", "c", "d"},
			valueType: nil,
			want:      "__a.b.c.d__",
		},
		{
			name:      "value type does not change output",
			key:       []string{"foo", "bar"},
			valueType: reflect.TypeOf(42),
			want:      "__foo.bar__",
		},
	}

	gen := NewDunderPlaceholders()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			flattenedKey, replaceVar := gen(tc.key, tc.valueType)
			if flattenedKey != tc.want {
				t.Errorf("flattenedKey: got %q, want %q", flattenedKey, tc.want)
			}
			if replaceVar != tc.want {
				t.Errorf("replaceVar: got %q, want %q", replaceVar, tc.want)
			}
		})
	}
}

// TestDunderMatchesDefaultPattern is the regex-contract test: every placeholder
// emitted by NewDunderPlaceholders must match DefaultPlaceholderPattern, so
// that a Configuration produced by EV2Mapping(_, NewDunderPlaceholders(), nil)
// is accepted by ValidatePipelineSchemaWithOptions(_, WithAllowPlaceholders("")).
// This test fails loudly if the producer or the validator's default pattern
// ever drift apart.
func TestDunderMatchesDefaultPattern(t *testing.T) {
	re, err := regexp.Compile(DefaultPlaceholderPattern)
	if err != nil {
		t.Fatalf("DefaultPlaceholderPattern is not a valid regexp: %v", err)
	}

	gen := NewDunderPlaceholders()
	keys := [][]string{
		{"top"},
		{"top", "nested"},
		{"top", "nested", "deeper"},
		{"with_underscore", "and.dots", "trailing"},
	}
	for _, k := range keys {
		_, v := gen(k, nil)
		if !re.MatchString(v) {
			t.Errorf("NewDunderPlaceholders(%v) produced %q which does not match DefaultPlaceholderPattern %q",
				k, v, DefaultPlaceholderPattern)
		}
	}
}

// TestEV2Mapping pins the map-only walker behavior. Scalar leaves (string,
// number, bool) are replaced with a placeholder; nested map[string]any values
// are recursed into; non-map non-string values (including []any) are treated
// as opaque scalars per the array-policy described on EV2Mapping's doc
// comment.
func TestEV2Mapping(t *testing.T) {
	tests := []struct {
		name              string
		input             types.Configuration
		expectedFlattened map[string]string
		expectedReplace   map[string]interface{}
	}{
		{
			name: "flat scalars",
			input: types.Configuration{
				"key1": "value1",
				"key2": 42,
				"key3": true,
			},
			expectedFlattened: map[string]string{
				"__key1__": "key1",
				"__key2__": "key2",
				"__key3__": "key3",
			},
			expectedReplace: map[string]interface{}{
				"key1": "__key1__",
				"key2": "__key2__",
				"key3": "__key3__",
			},
		},
		{
			name: "nested maps recurse",
			input: types.Configuration{
				"parent": map[string]interface{}{
					"nested":    "nestedvalue",
					"nestedInt": 42,
					"deeper": map[string]interface{}{
						"deepest": "deepestvalue",
					},
				},
			},
			expectedFlattened: map[string]string{
				"__parent.nested__":         "parent.nested",
				"__parent.nestedInt__":      "parent.nestedInt",
				"__parent.deeper.deepest__": "parent.deeper.deepest",
			},
			expectedReplace: map[string]interface{}{
				"parent": map[string]interface{}{
					"nested":    "__parent.nested__",
					"nestedInt": "__parent.nestedInt__",
					"deeper": map[string]interface{}{
						"deepest": "__parent.deeper.deepest__",
					},
				},
			},
		},
		{
			name: "mixed flat and nested",
			input: types.Configuration{
				"key1": "value1",
				"parent": map[string]interface{}{
					"nested": "nestedvalue",
				},
			},
			expectedFlattened: map[string]string{
				"__key1__":          "key1",
				"__parent.nested__": "parent.nested",
			},
			expectedReplace: map[string]interface{}{
				"key1": "__key1__",
				"parent": map[string]interface{}{
					"nested": "__parent.nested__",
				},
			},
		},
		{
			name: "slice values are treated as opaque scalars (no per-element placeholders)",
			input: types.Configuration{
				"registries": []any{"quay.io", "registry.redhat.io"},
			},
			expectedFlattened: map[string]string{
				"__registries__": "registries",
			},
			expectedReplace: map[string]interface{}{
				"registries": "__registries__",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			flattened, replace := EV2Mapping(tc.input, NewDunderPlaceholders(), nil)
			if diff := cmp.Diff(tc.expectedFlattened, flattened); diff != "" {
				t.Errorf("flattened mismatch (-want +got):\n%s", diff)
			}
			if diff := cmp.Diff(tc.expectedReplace, replace); diff != "" {
				t.Errorf("replace mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

// TestEV2MappingPrefix verifies that a non-nil prefix seeds the dotted path
// for every emitted placeholder.
func TestEV2MappingPrefix(t *testing.T) {
	input := types.Configuration{
		"key": "value",
	}
	flattened, replace := EV2Mapping(input, NewDunderPlaceholders(), []string{"root", "branch"})
	wantFlat := map[string]string{
		"__root.branch.key__": "root.branch.key",
	}
	wantReplace := map[string]interface{}{
		"key": "__root.branch.key__",
	}
	if diff := cmp.Diff(wantFlat, flattened); diff != "" {
		t.Errorf("flattened mismatch (-want +got):\n%s", diff)
	}
	if diff := cmp.Diff(wantReplace, replace); diff != "" {
		t.Errorf("replace mismatch (-want +got):\n%s", diff)
	}
}

// TestEV2MappingDoesNotMutatePrefix guards against a regression where the
// recursive walker appends to a shared backing array, leaking sibling-key
// suffixes into nested placeholders. With map-iteration order randomised, a
// shared-prefix bug would surface as flaky test failures rather than a
// deterministic one — running the walker many times against a single tree
// makes a regression overwhelmingly likely to be caught.
func TestEV2MappingDoesNotMutatePrefix(t *testing.T) {
	input := types.Configuration{
		"alpha": "a",
		"beta":  "b",
		"gamma": map[string]any{"delta": "d"},
	}
	want := map[string]string{
		"__alpha__":       "alpha",
		"__beta__":        "beta",
		"__gamma.delta__": "gamma.delta",
	}
	for i := 0; i < 32; i++ {
		got, _ := EV2Mapping(input, NewDunderPlaceholders(), nil)
		if diff := cmp.Diff(want, got); diff != "" {
			t.Fatalf("iteration %d: flattened mismatch (-want +got):\n%s", i, diff)
		}
	}
}
