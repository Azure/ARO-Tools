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
	"encoding/json"
	"fmt"

	"github.com/santhosh-tekuri/jsonschema/v6"

	"sigs.k8s.io/yaml"

	_ "embed"
)

//go:embed pipeline.schema.v1.json
var pipelineSchemaV1Content []byte
var pipelineSchemaV1Ref = "pipeline.schema.v1"
var defaultSchemaRef = pipelineSchemaV1Ref

func getSchemaForPipeline(pipelineMap map[string]interface{}) (pipelineSchema *jsonschema.Schema, schemaRef string, err error) {
	schemaRef, _ = pipelineMap["$schema"].(string)
	return getSchemaForRef(schemaRef)
}

func getSchemaForRef(schemaRef string) (*jsonschema.Schema, string, error) {
	return getSchemaForRefWithOptions(schemaRef, nil)
}

// getSchemaForRefWithOptions resolves a schema reference like getSchemaForRef
// does, but threads validateOptions through so that placeholder-mode widening
// can be applied between decoding and compilation.
func getSchemaForRefWithOptions(schemaRef string, opts *validateOptions) (*jsonschema.Schema, string, error) {
	if schemaRef == "" {
		schemaRef = defaultSchemaRef
	}
	switch schemaRef {
	case pipelineSchemaV1Ref:
		schemaMap, err := decodeSchemaMap(pipelineSchemaV1Content)
		if err != nil {
			return nil, schemaRef, err
		}
		if opts != nil && opts.allowPlaceholders != "" {
			widenScalarsForPlaceholders(schemaMap, opts.allowPlaceholders)
		}
		pipelineSchema, err := compileSchemaFromMap(schemaRef, schemaMap)
		return pipelineSchema, schemaRef, err
	default:
		return nil, "", fmt.Errorf("unsupported schema reference: %s", schemaRef)
	}
}

// ValidatePipelineSchema validates pipelineContent against the schema declared
// by its "$schema" key (or the default v1 schema if none is set). Strict
// validation: every type must match exactly. To allow opt-in placeholder
// strings on non-string scalar fields, use ValidatePipelineSchemaWithOptions
// together with WithAllowPlaceholders.
func ValidatePipelineSchema(pipelineContent []byte) error {
	return ValidatePipelineSchemaWithOptions(pipelineContent)
}

// ValidatePipelineSchemaWithOptions validates pipelineContent against the
// declared (or default) schema. When opts include WithAllowPlaceholders, the
// schema is widened in-memory so that non-string scalar fields also accept
// strings matching the configured placeholder pattern. The schema asset on
// disk and the strict default validation paths are not affected.
//
// This is intended for callers that validate pipeline.yaml templates against
// a fully dunderized configuration (e.g. sdp-pipelines). EV2 manifest
// generation and other strict callers should continue to use
// ValidatePipelineSchema.
func ValidatePipelineSchemaWithOptions(pipelineContent []byte, opts ...ValidateOption) error {
	resolved := newValidateOptions(opts...)

	pipelineMap := make(map[string]interface{})
	if err := yaml.Unmarshal(pipelineContent, &pipelineMap); err != nil {
		return fmt.Errorf("failed to unmarshal pipeline YAML content: %v", err)
	}

	schemaRef, _ := pipelineMap["$schema"].(string)
	pipelineSchema, schemaRef, err := getSchemaForRefWithOptions(schemaRef, resolved)
	if err != nil {
		return fmt.Errorf("failed to load pipeline schema: %v", err)
	}

	if err := pipelineSchema.Validate(pipelineMap); err != nil {
		return fmt.Errorf("pipeline is not compliant with schema %s: %v", schemaRef, err)
	}
	return nil
}

// decodeSchemaMap unmarshals a JSON schema asset into a generic map so that
// callers can inspect or rewrite it before compilation.
func decodeSchemaMap(schemaBytes []byte) (map[string]interface{}, error) {
	schemaMap := make(map[string]interface{})
	if err := json.Unmarshal(schemaBytes, &schemaMap); err != nil {
		return nil, fmt.Errorf("failed to unmarshal schema content: %v", err)
	}
	return schemaMap, nil
}

// compileSchemaFromMap compiles a schema previously decoded with
// decodeSchemaMap (and possibly rewritten by the caller) into a jsonschema.Schema.
func compileSchemaFromMap(schemaRef string, schemaMap map[string]interface{}) (*jsonschema.Schema, error) {
	c := jsonschema.NewCompiler()
	if err := c.AddResource(schemaRef, schemaMap); err != nil {
		return nil, fmt.Errorf("failed to add schema resource %s: %v", schemaRef, err)
	}
	pipelineSchema, err := c.Compile(schemaRef)
	if err != nil {
		return nil, fmt.Errorf("failed to compile schema %s: %v", schemaRef, err)
	}
	return pipelineSchema, nil
}

type AdoArtifactDownloadPipelineReference struct {
	ADOProject   string `json:"adoProject,omitempty"`
	ArtifactName string `json:"artifactName,omitempty"`
	BuildID      string `json:"buildId,omitempty"`

	// FileSourceToDestination is a mapping of source file paths within the artifact to destination file paths in the local filesystem.
	FileSourceToDestination map[string]string `json:"fileSourceToDestination,omitempty"`
}
