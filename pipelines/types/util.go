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
	if schemaRef == "" {
		schemaRef = defaultSchemaRef
	}
	switch schemaRef {
	case pipelineSchemaV1Ref:
		pipelineSchema, err := compileSchema(schemaRef, pipelineSchemaV1Content)
		return pipelineSchema, schemaRef, err
	default:
		return nil, "", fmt.Errorf("unsupported schema reference: %s", schemaRef)
	}
}

func ValidatePipelineSchema(pipelineContent []byte) error {
	// unmarshal pipeline content
	pipelineMap := make(map[string]interface{})
	err := yaml.Unmarshal(pipelineContent, &pipelineMap)
	if err != nil {
		return fmt.Errorf("failed to unmarshal pipeline YAML content: %v", err)
	}

	// load pipeline schema
	pipelineSchema, schemaRef, err := getSchemaForPipeline(pipelineMap)
	if err != nil {
		return fmt.Errorf("failed to load pipeline schema: %v", err)
	}

	// validate pipeline schema
	err = pipelineSchema.Validate(pipelineMap)
	if err != nil {
		return fmt.Errorf("pipeline is not compliant with schema %s: %v", schemaRef, err)
	}
	return nil
}

// compileSchema decodes a JSON schema asset and compiles it. It is preserved
// as a wrapper around decodeSchemaMap + compileSchemaFromMap so that other
// code paths can rewrite the schema map between decode and compile.
func compileSchema(schemaRef string, schemaBytes []byte) (*jsonschema.Schema, error) {
	schemaMap, err := decodeSchemaMap(schemaBytes)
	if err != nil {
		return nil, err
	}
	return compileSchemaFromMap(schemaRef, schemaMap)
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
