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
	"testing"
)

// TestWidenScalarsForPlaceholders_NodeShapes exercises widenScalarsForPlaceholders
// against synthetic schema fragments to verify each non-string scalar shape is
// either wrapped in a oneOf or left alone. The end-to-end behavior is covered
// by TestValidatePipelineSchemaPlaceholderMode; this test pins the structural
// rewrite invariants directly so a future refactor cannot accidentally drop a
// branch (in particular, the non-string const branch added in response to a
// review request).
func TestWidenScalarsForPlaceholders_NodeShapes(t *testing.T) {
	pattern := DefaultPlaceholderPattern

	cases := []struct {
		name     string
		input    map[string]interface{}
		wrapped  bool
		notation string
	}{
		{
			name:     "boolean_type_is_widened",
			input:    map[string]interface{}{"type": "boolean"},
			wrapped:  true,
			notation: "{type: boolean}",
		},
		{
			name:     "string_type_is_unchanged",
			input:    map[string]interface{}{"type": "string"},
			wrapped:  false,
			notation: "{type: string}",
		},
		{
			name: "non_string_enum_is_widened",
			input: map[string]interface{}{
				"enum": []interface{}{float64(1), float64(2), float64(3)},
			},
			wrapped:  true,
			notation: "{enum: [1,2,3]}",
		},
		{
			name: "string_enum_is_unchanged",
			input: map[string]interface{}{
				"enum": []interface{}{"a", "b"},
			},
			wrapped:  false,
			notation: `{enum: ["a","b"]}`,
		},
		{
			name:     "boolean_const_is_widened",
			input:    map[string]interface{}{"const": true},
			wrapped:  true,
			notation: "{const: true}",
		},
		{
			name:     "integer_const_is_widened",
			input:    map[string]interface{}{"const": float64(42)},
			wrapped:  true,
			notation: "{const: 42}",
		},
		{
			name:     "string_const_is_unchanged",
			input:    map[string]interface{}{"const": "ImageMirror"},
			wrapped:  false,
			notation: `{const: "ImageMirror"}`,
		},
		{
			name: "typed_boolean_const_is_widened",
			input: map[string]interface{}{
				"type":  "boolean",
				"const": true,
			},
			wrapped:  true,
			notation: "{type: boolean, const: true}",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Build a tiny root schema with the input under a property so the
			// walker actually descends into it.
			root := map[string]interface{}{
				"properties": map[string]interface{}{
					"x": tc.input,
				},
			}
			widenScalarsForPlaceholders(root, pattern)

			props := root["properties"].(map[string]interface{})
			x := props["x"].(map[string]interface{})
			_, isOneOf := x["oneOf"]
			if tc.wrapped && !isOneOf {
				t.Fatalf("expected %s to be wrapped in oneOf, got: %#v", tc.notation, x)
			}
			if !tc.wrapped && isOneOf {
				t.Fatalf("expected %s to be left unchanged, got: %#v", tc.notation, x)
			}
		})
	}
}

// TestWidenScalarsForPlaceholders_ConstRefBoundary asserts the widener
// resolves a $ref to a definition whose schema body is a non-string const
// (the imageMirrorStep schema uses this shape via anyOf alternatives that
// constrain publicSource to "const: true"). The synthetic fixture mirrors the
// reachability shape that a real refactor of pipeline.schema.v1.json could
// introduce so the invariant is pinned regardless of upstream changes.
func TestWidenScalarsForPlaceholders_ConstRefBoundary(t *testing.T) {
	root := map[string]interface{}{
		"definitions": map[string]interface{}{
			"trueConst": map[string]interface{}{"const": true},
		},
		"properties": map[string]interface{}{
			"flag": map[string]interface{}{
				"$ref": "#/definitions/trueConst",
			},
		},
	}
	widenScalarsForPlaceholders(root, DefaultPlaceholderPattern)

	defs := root["definitions"].(map[string]interface{})
	target := defs["trueConst"].(map[string]interface{})
	if _, isOneOf := target["oneOf"]; !isOneOf {
		t.Fatalf("expected {const: true} target of $ref to be wrapped in oneOf, got: %#v", target)
	}
}
