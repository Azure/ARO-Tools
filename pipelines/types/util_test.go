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
	"strings"
	"testing"
)

// pipelineWithStepFields returns a minimal valid pipeline.yaml whose single
// Shell step has the given extra top-level keys injected (verbatim, indented
// to match the existing four-space step body indentation).
func pipelineWithStepFields(extraStepLines string) string {
	indented := ""
	if extraStepLines != "" {
		for _, line := range strings.Split(strings.TrimRight(extraStepLines, "\n"), "\n") {
			indented += "    " + line + "\n"
		}
	}
	return `$schema: pipeline.schema.v1
serviceGroup: Microsoft.Azure.ARO.Test
rolloutName: Test Rollout
resourceGroups:
- name: test
  resourceGroup: test-rg
  subscription: test-sub
  steps:
  - name: deploy
    action: Shell
    command: echo hello
` + indented
}

// pipelineWithResourceGroupFields embeds extra top-level keys on the only
// resourceGroup entry (e.g. executionConstraints arrays).
func pipelineWithResourceGroupFields(extraRGLines string) string {
	indented := ""
	for _, line := range strings.Split(strings.TrimRight(extraRGLines, "\n"), "\n") {
		indented += "  " + line + "\n"
	}
	return `$schema: pipeline.schema.v1
serviceGroup: Microsoft.Azure.ARO.Test
rolloutName: Test Rollout
resourceGroups:
- name: test
  resourceGroup: test-rg
  subscription: test-sub
` + indented + `  steps:
  - name: deploy
    action: Shell
    command: echo hello
`
}

func TestValidatePipelineSchemaPlaceholderMode(t *testing.T) {
	tests := []struct {
		name        string
		yaml        string
		opts        []ValidateOption
		expectError bool
		description string
	}{
		{
			name:        "strict_rejects_arbitrary_string_on_boolean",
			yaml:        pipelineWithStepFields("omitFromServiceGroupCompletion: \"definitely-not-a-bool\"\n"),
			opts:        nil,
			expectError: true,
			description: "strict mode must reject a non-bool value on a boolean field",
		},
		{
			name:        "strict_rejects_placeholder_on_boolean",
			yaml:        pipelineWithStepFields("omitFromServiceGroupCompletion: __rg.omit__\n"),
			opts:        nil,
			expectError: true,
			description: "strict mode must reject placeholder string on a boolean field (regression guard)",
		},
		{
			name:        "placeholder_accepts_placeholder_on_boolean",
			yaml:        pipelineWithStepFields("omitFromServiceGroupCompletion: __rg.omit__\n"),
			opts:        []ValidateOption{WithAllowPlaceholders("")},
			expectError: false,
			description: "placeholder mode must accept a dunder string on a boolean field",
		},
		{
			name:        "placeholder_rejects_non_matching_string_on_boolean",
			yaml:        pipelineWithStepFields("omitFromServiceGroupCompletion: not_a_placeholder\n"),
			opts:        []ValidateOption{WithAllowPlaceholders("")},
			expectError: true,
			description: "placeholder mode must still reject arbitrary strings that do not match the pattern",
		},
		{
			name:        "placeholder_accepts_real_bool",
			yaml:        pipelineWithStepFields("omitFromServiceGroupCompletion: true\n"),
			opts:        []ValidateOption{WithAllowPlaceholders("")},
			expectError: false,
			description: "placeholder mode must still accept genuine boolean values",
		},
		{
			name:        "placeholder_accepts_placeholder_on_nested_integer",
			yaml:        pipelineWithStepFields("automatedRetry:\n  maximumRetryCount: __retry.count__\n"),
			opts:        []ValidateOption{WithAllowPlaceholders("")},
			expectError: false,
			description: "placeholder mode must accept dunder strings on integer fields nested under stepMeta",
		},
		{
			name:        "placeholder_accepts_real_integer",
			yaml:        pipelineWithStepFields("automatedRetry:\n  maximumRetryCount: 5\n"),
			opts:        []ValidateOption{WithAllowPlaceholders("")},
			expectError: false,
			description: "placeholder mode must still accept genuine integer values",
		},
		{
			name:        "placeholder_accepts_singleton_via_ref",
			yaml:        pipelineWithResourceGroupFields("executionConstraints:\n  - singleton: __ec.singleton__\n"),
			opts:        []ValidateOption{WithAllowPlaceholders("")},
			expectError: false,
			description: "placeholder mode must reach scalars defined under #/definitions referenced via $ref",
		},
		{
			name:        "string_field_unchanged_strict",
			yaml:        pipelineWithStepFields(""),
			opts:        nil,
			expectError: false,
			description: "strict mode accepts a minimal pipeline whose string fields are real strings",
		},
		{
			name:        "string_field_unchanged_placeholder",
			yaml:        pipelineWithStepFields(""),
			opts:        []ValidateOption{WithAllowPlaceholders("")},
			expectError: false,
			description: "placeholder mode leaves string-typed fields strict (real strings accepted)",
		},
		{
			name:        "custom_pattern_is_honored",
			yaml:        pipelineWithStepFields("omitFromServiceGroupCompletion: <<rg.omit>>\n"),
			opts:        []ValidateOption{WithAllowPlaceholders("^<<.+>>$")},
			expectError: false,
			description: "a custom pattern supplied via WithAllowPlaceholders is the one used to match placeholders",
		},
		{
			name:        "custom_pattern_rejects_default_dunder",
			yaml:        pipelineWithStepFields("omitFromServiceGroupCompletion: __rg.omit__\n"),
			opts:        []ValidateOption{WithAllowPlaceholders("^<<.+>>$")},
			expectError: true,
			description: "a custom pattern rules out the default dunder convention",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidatePipelineSchemaWithOptions([]byte(tt.yaml), tt.opts...)
			if tt.expectError && err == nil {
				t.Fatalf("%s: expected an error, got nil", tt.description)
			}
			if !tt.expectError && err != nil {
				t.Fatalf("%s: expected no error, got: %v", tt.description, err)
			}
		})
	}
}

// TestValidatePipelineSchemaBackwardsCompatible asserts the legacy
// ValidatePipelineSchema entry point retains strict behavior even after the
// option plumbing was added.
func TestValidatePipelineSchemaBackwardsCompatible(t *testing.T) {
	yaml := pipelineWithStepFields("omitFromServiceGroupCompletion: __rg.omit__\n")
	if err := ValidatePipelineSchema([]byte(yaml)); err == nil {
		t.Fatalf("ValidatePipelineSchema (strict) must reject placeholder strings on boolean fields, got nil error")
	}
}
