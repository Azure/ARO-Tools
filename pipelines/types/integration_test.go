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

	"github.com/Azure/ARO-Tools/config"
	"github.com/Azure/ARO-Tools/config/types"
)

// pipelineTemplateWithScalarRefs is a pipeline.yaml template that exercises
// every non-string scalar field that pipeline.schema.v1 declares on a Shell
// step (omitFromServiceGroupCompletion: bool, automatedRetry.maximumRetryCount:
// integer) plus a string field. It is rendered twice in the end-to-end test:
// once with a "dunder configuration" produced by EV2Mapping(NewDunderPlaceholders),
// once with the real typed configuration, so the placeholder + strict modes
// can be compared head-to-head.
const pipelineTemplateWithScalarRefs = `$schema: pipeline.schema.v1
serviceGroup: Microsoft.Azure.ARO.Test
rolloutName: Test Rollout
resourceGroups:
- name: test
  resourceGroup: {{ .rg.name }}
  subscription: {{ .rg.sub }}
  steps:
  - name: deploy
    action: Shell
    command: echo hello
    omitFromServiceGroupCompletion: {{ .step.omit }}
    automatedRetry:
      maximumRetryCount: {{ .step.retry }}
`

// TestEndToEndPlaceholderValidation is the producer + validator integration
// test asked for in the PR #237 review. It demonstrates the contract that
// EV2Mapping(NewDunderPlaceholders) (producer, moved into this package by
// this PR) and ValidatePipelineSchemaWithOptions(WithAllowPlaceholders)
// (validator, added by this PR) agree on the same placeholder convention.
//
// The flow tested mirrors the sdp-pipelines dunder precompile pass:
//
//  1. A typed configuration (real values) is run through EV2Mapping with the
//     dunder generator to produce a dunder configuration whose every leaf is
//     replaced with a "__path__" placeholder string.
//  2. A pipeline.yaml template referencing scalar fields (string, bool,
//     integer) is rendered with that dunder configuration; the resulting YAML
//     carries placeholder strings in fields the schema declares as
//     non-string scalars.
//  3. ValidatePipelineSchemaWithOptions(rendered, WithAllowPlaceholders(""))
//     accepts that YAML.
//
// The test also pins two regression guards: strict-mode validation must
// reject the same dunderized YAML (so placeholder mode never becomes the
// default), and rendering the same template with the real typed
// configuration must always pass strict-mode validation (so the EV2Mapping
// move-in did not change non-placeholder rendering behavior).
//
// Scope note: this test exercises the schema-validation surface only, which
// is the exact surface PR #237 changes. NewPipelineFromBytes additionally
// unmarshals the rendered YAML into the typed *Pipeline struct, and that
// unmarshal step is unchanged by this PR — callers that pass placeholder
// strings into non-string Go fields will still see unmarshal errors. The
// sdp-pipelines call site that opts into placeholder mode must therefore use
// the rendered-bytes form (ValidatePipelineSchemaWithOptions, exercised
// here) rather than the typed-struct form (NewPipelineFromBytes) for its
// dunder precompile pass. The real-config strict path through
// NewPipelineFromBytes is exercised below to confirm ordinary (non-dunder)
// usage remains intact.
//
// Per ARO-HCP configuration policy
// (https://github.com/Azure/ARO-HCP/blob/main/docs/configuration.md#limitations),
// arrays are not supported in per-region configuration, so the fixture uses
// scalar leaves only.
func TestEndToEndPlaceholderValidation(t *testing.T) {
	typedConfig := types.Configuration{
		"rg": map[string]any{
			"name": "test-rg",
			"sub":  "test-sub",
		},
		"step": map[string]any{
			"omit":  true,
			"retry": 3,
		},
	}

	flat, dunderRaw := EV2Mapping(typedConfig, NewDunderPlaceholders(), nil)

	// Producer self-check: every leaf in the typed configuration must produce
	// a corresponding entry in the flat map, including the non-string-scalar
	// leaves that placeholder mode exists to admit.
	for _, want := range []string{"__rg.name__", "__rg.sub__", "__step.omit__", "__step.retry__"} {
		if _, ok := flat[want]; !ok {
			t.Fatalf("EV2Mapping flat map missing %q; got keys %v", want, keysOf(flat))
		}
	}

	dunderCfg := types.Configuration(dunderRaw)

	dunderBytes, err := config.PreprocessContent([]byte(pipelineTemplateWithScalarRefs), dunderCfg)
	if err != nil {
		t.Fatalf("PreprocessContent(dunderCfg) must succeed, got: %v", err)
	}

	// Validator self-check: the dunder-rendered YAML must actually contain
	// placeholder strings in non-string scalar positions. Otherwise the
	// subsequent placeholder/strict assertions wouldn't distinguish the two
	// modes.
	dunderYAML := string(dunderBytes)
	for _, want := range []string{"omitFromServiceGroupCompletion: __step.omit__", "maximumRetryCount: __step.retry__"} {
		if !strings.Contains(dunderYAML, want) {
			t.Fatalf("expected dunder-rendered YAML to contain %q; got:\n%s", want, dunderYAML)
		}
	}

	realBytes, err := config.PreprocessContent([]byte(pipelineTemplateWithScalarRefs), typedConfig)
	if err != nil {
		t.Fatalf("PreprocessContent(typedConfig) must succeed, got: %v", err)
	}

	// 1) Placeholder mode accepts the dunder-rendered YAML.
	if err := ValidatePipelineSchemaWithOptions(dunderBytes, WithAllowPlaceholders("")); err != nil {
		t.Errorf("placeholder mode must accept dunder-rendered YAML, got: %v", err)
	}

	// 2) Strict mode rejects the same dunder-rendered YAML (regression guard
	//    so placeholder mode never becomes the default).
	if err := ValidatePipelineSchemaWithOptions(dunderBytes); err == nil {
		t.Errorf("strict mode must reject placeholder strings on non-string scalar fields")
	}

	// 3) Strict mode accepts the typed-rendered YAML (regression guard so the
	//    EV2Mapping move-in did not change non-placeholder behavior).
	if err := ValidatePipelineSchemaWithOptions(realBytes); err != nil {
		t.Errorf("strict mode must accept typed-rendered YAML, got: %v", err)
	}

	// 4) Full NewPipelineFromBytes flow (validate + unmarshal + Validate) is
	//    intact for the typed-configuration path — proves the move-in did
	//    not regress ordinary production rendering.
	if _, err := NewPipelineFromBytes([]byte(pipelineTemplateWithScalarRefs), typedConfig); err != nil {
		t.Errorf("NewPipelineFromBytes(typedConfig) must succeed, got: %v", err)
	}
}

func keysOf(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
