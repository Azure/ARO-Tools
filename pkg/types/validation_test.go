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

	"gotest.tools/v3/assert"

	"sigs.k8s.io/yaml"
)

func TestRGValidate(t *testing.T) {
	testCases := []struct {
		name string
		rg   *ResourceGroup
		err  string
	}{
		{
			name: "missing name",
			rg:   &ResourceGroup{},
			err:  "resource group name is required",
		},
		{
			name: "missing subscription",
			rg:   &ResourceGroup{Name: "test"},
			err:  "subscription is required",
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.rg.Validate()
			assert.Error(t, err, tc.err)
		})
	}

}

func TestPipelineValidate(t *testing.T) {
	testCases := []struct {
		name     string
		pipeline *Pipeline
		err      string
	}{
		{
			name: "missing name",
			pipeline: &Pipeline{
				ResourceGroups: []*ResourceGroup{{}},
			},
			err: "resource group name is required",
		},
		{
			name: "missing subscription",
			pipeline: &Pipeline{
				ResourceGroups: []*ResourceGroup{
					{
						Name: "rg",
					},
				},
			},
			err: "subscription is required",
		},
		{
			name: "missing step dependency",
			pipeline: &Pipeline{
				ResourceGroups: []*ResourceGroup{
					{
						Name:          "rg1",
						ResourceGroup: "rg1",
						Subscription:  "sub1",
						Steps: []Step{
							NewShellStep("step1", "echo foo"),
						},
					},
					{
						Name:         "rg2",
						Subscription: "sub1",
						Steps: []Step{
							NewShellStep("step2", "echo bar").WithDependsOn(StepDependency{ResourceGroup: "rg1", Step: "step3"}),
						},
					},
				},
			},
			err: "pipeline.resourceGroups[1:rg2].steps[0:step2]: dependency rg1/step3 invalid: resource group rg1 has no step step3",
		},
		{
			name: "duplicate step name",
			pipeline: &Pipeline{
				ResourceGroups: []*ResourceGroup{
					{
						Name:          "rg1",
						ResourceGroup: "rg1",
						Subscription:  "sub1",
						Steps: []Step{
							NewShellStep("step1", "echo foo"),
							NewShellStep("step1", "echo bar").WithDependsOn(StepDependency{ResourceGroup: "rg1", Step: "step1"}),
						},
					},
				},
			},
			err: `pipeline.resourceGroups[0:rg1].steps[1:step1]: step name "step1" duplicated`,
		},
		{
			name: "same step name across groups",
			pipeline: &Pipeline{
				ResourceGroups: []*ResourceGroup{
					{
						Name:          "rg1",
						ResourceGroup: "rg1",
						Subscription:  "sub1",
						Steps: []Step{
							NewShellStep("step1", "echo foo"),
						},
					},
					{
						Name:         "rg2",
						Subscription: "sub1",
						Steps: []Step{
							NewShellStep("step1", "echo bar").WithDependsOn(StepDependency{ResourceGroup: "rg1", Step: "step1"}),
						},
					},
				},
			},
		},
		{
			name: "valid step dependencies",
			pipeline: &Pipeline{
				ResourceGroups: []*ResourceGroup{
					{
						Name:          "rg1",
						ResourceGroup: "rg1",
						Subscription:  "sub1",
						Steps: []Step{
							NewShellStep("step1", "echo foo"),
						},
					},
					{
						Name:          "rg2",
						ResourceGroup: "rg2",
						Subscription:  "sub1",
						Steps: []Step{
							NewShellStep("step2", "echo bar").WithDependsOn(StepDependency{ResourceGroup: "rg1", Step: "step1"}),
						},
					},
				},
			},
			err: "",
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.pipeline.Validate()
			if tc.err == "" {
				assert.NilError(t, err)
			} else {
				assert.Error(t, err, tc.err)
			}
		})
	}
}

func TestGetSchemaForPipeline(t *testing.T) {
	testCases := []struct {
		name              string
		pipeline          map[string]interface{}
		expectedSchemaRef string
		err               string
	}{
		{
			name:              "default schema",
			pipeline:          map[string]interface{}{},
			expectedSchemaRef: defaultSchemaRef,
		},
		{
			name: "explicit schema",
			pipeline: map[string]interface{}{
				"$schema": pipelineSchemaV1Ref,
			},
			expectedSchemaRef: pipelineSchemaV1Ref,
		},
		{
			name: "invalid schema",
			pipeline: map[string]interface{}{
				"$schema": "invalid",
			},
			expectedSchemaRef: "",
			err:               "unsupported schema reference: invalid",
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			schema, ref, err := getSchemaForPipeline(tc.pipeline)
			if tc.err == "" {
				assert.NilError(t, err)
				assert.Assert(t, schema != nil)
				if tc.expectedSchemaRef != "" {
					assert.Equal(t, ref, tc.expectedSchemaRef)
				}
			} else {
				assert.Error(t, err, tc.err)
			}
		})
	}
}

func TestValidatePipelineSchema(t *testing.T) {
	testCases := []struct {
		name              string
		pipeline          map[string]interface{}
		expectedSchemaRef string
		err               string
	}{
		{
			name: "valid shell",
			pipeline: map[string]interface{}{
				"serviceGroup": "test",
				"rolloutName":  "test",
				"resourceGroups": []interface{}{
					map[string]interface{}{
						"name":          "rg",
						"resourceGroup": "rg",
						"subscription":  "sub",
						"steps": []interface{}{
							map[string]interface{}{
								"name":    "step",
								"action":  "Shell",
								"command": "echo hello",
							},
						},
					},
				},
			},
		},
		{
			name: "invalid",
			pipeline: map[string]interface{}{
				"serviceGroup": "test",
				"rolloutName":  "test",
				"resourceGroups": []interface{}{
					map[string]interface{}{
						"name":          "rg",
						"resourceGroup": "rg",
						"subscription":  "sub",
						"aksCluster":    "aks",
						"steps": []interface{}{
							map[string]interface{}{
								"name":   "step",
								"action": "Shell",
							},
						},
					},
				},
			},
			err: "pipeline is not compliant with schema pipeline.schema.v1",
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			pipelineBytes, err := yaml.Marshal(tc.pipeline)
			assert.NilError(t, err)
			err = ValidatePipelineSchema(pipelineBytes)
			if tc.err == "" {
				assert.NilError(t, err)
			} else {
				assert.ErrorContains(t, err, tc.err)
			}
		})
	}
}
