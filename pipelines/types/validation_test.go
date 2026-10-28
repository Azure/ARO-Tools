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
			rg:   &ResourceGroup{ResourceGroupMeta: &ResourceGroupMeta{}},
			err:  "resource group name is required",
		},
		{
			name: "missing subscription",
			rg:   &ResourceGroup{ResourceGroupMeta: &ResourceGroupMeta{Name: "test"}},
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
				ResourceGroups: []*ResourceGroup{{ResourceGroupMeta: &ResourceGroupMeta{}}},
			},
			err: "resource group name is required",
		},
		{
			name: "missing subscription",
			pipeline: &Pipeline{
				ResourceGroups: []*ResourceGroup{
					{
						ResourceGroupMeta: &ResourceGroupMeta{Name: "rg"},
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
						ResourceGroupMeta: &ResourceGroupMeta{
							Name:          "rg1",
							ResourceGroup: "rg1",
							Subscription:  "sub1",
						},
						Steps: []Step{
							&ShellStep{
								StepMeta: StepMeta{
									Name:   "step1",
									Action: "Shell",
								},
								Command: "echo foo",
							},
						},
					},
					{
						ResourceGroupMeta: &ResourceGroupMeta{
							Name:         "rg2",
							Subscription: "sub1",
						},
						Steps: []Step{
							&ShellStep{
								StepMeta: StepMeta{
									Name:      "step2",
									Action:    "Shell",
									DependsOn: []StepDependency{{ResourceGroup: "rg1", Step: "step3"}},
								},
								Command: "echo bar",
							},
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
						ResourceGroupMeta: &ResourceGroupMeta{
							Name:          "rg1",
							ResourceGroup: "rg1",
							Subscription:  "sub1",
						},
						Steps: []Step{
							&ShellStep{
								StepMeta: StepMeta{
									Name:   "step1",
									Action: "Shell",
								},
								Command: "echo foo",
							},
							&ShellStep{
								StepMeta: StepMeta{
									Name:      "step1",
									Action:    "Shell",
									DependsOn: []StepDependency{{ResourceGroup: "rg1", Step: "step1"}},
								},
								Command: "echo bar",
							},
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
						ResourceGroupMeta: &ResourceGroupMeta{
							Name:          "rg1",
							ResourceGroup: "rg1",
							Subscription:  "sub1",
						},
						Steps: []Step{
							&ShellStep{
								StepMeta: StepMeta{
									Name:   "step1",
									Action: "Shell",
								},
								Command: "echo foo",
							},
						},
					},
					{
						ResourceGroupMeta: &ResourceGroupMeta{
							Name:         "rg2",
							Subscription: "sub1",
						},
						Steps: []Step{
							&ShellStep{
								StepMeta: StepMeta{
									Name:      "step1",
									Action:    "Shell",
									DependsOn: []StepDependency{{ResourceGroup: "rg1", Step: "step1"}},
								},
								Command: "echo bar",
							},
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
						ResourceGroupMeta: &ResourceGroupMeta{
							Name:          "rg1",
							ResourceGroup: "rg1",
							Subscription:  "sub1",
						},
						Steps: []Step{
							&ShellStep{
								StepMeta: StepMeta{
									Name:   "step1",
									Action: "Shell",
								},
								Command: "echo foo",
							},
						},
					},
					{
						ResourceGroupMeta: &ResourceGroupMeta{
							Name:          "rg2",
							ResourceGroup: "rg2",
							Subscription:  "sub1",
						},
						Steps: []Step{
							&ShellStep{
								StepMeta: StepMeta{
									Name:      "step2",
									Action:    "Shell",
									DependsOn: []StepDependency{{ResourceGroup: "rg1", Step: "step1"}},
								},
								Command: "echo bar",
							},
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
								"shellIdentity": map[string]interface{}{
									"value": "test-msi",
								},
							},
						},
					},
				},
			},
		},
		{
			name: "valid shell timeout",
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
								"timeout": "75m",
								"shellIdentity": map[string]interface{}{
									"value": "test-msi",
								},
							},
						},
					},
				},
			},
		},
		{
			name: "safefly is not authorable",
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
								"name":   "step",
								"action": "SafeFly",
								"shellIdentity": map[string]interface{}{
									"configRef": "safeFlyMsiId",
								},
							},
						},
					},
				},
			},
			err: "pipeline is not compliant with schema pipeline.schema.v1",
		},
		{
			name: "invalid shell timeout",
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
								"timeout": "tomorrow",
							},
						},
					},
				},
			},
			err: "pipeline is not compliant with schema pipeline.schema.v1",
		},
		{
			name: "valid istio upgrade",
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
								"action": "IstioUpgrade",
								"aksCluster": map[string]interface{}{
									"configRef": "svc.aks.name",
								},
							},
						},
					},
				},
			},
		},
		{
			name: "valid istio upgrade with optional fields",
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
								"action": "IstioUpgrade",
								"aksCluster": map[string]interface{}{
									"configRef": "svc.aks.name",
								},
								"timeout": "75m",
								"identityFrom": map[string]interface{}{
									"resourceGroup": "rg",
									"step":          "output",
									"name":          "globalMSIId",
								},
							},
						},
					},
				},
			},
		},
		{
			name: "invalid istio upgrade missing aksCluster",
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
								"action": "IstioUpgrade",
							},
						},
					},
				},
			},
			err: "pipeline is not compliant with schema pipeline.schema.v1",
		},
		{
			name: "invalid istio upgrade bad timeout",
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
								"action": "IstioUpgrade",
								"aksCluster": map[string]interface{}{
									"configRef": "svc.aks.name",
								},
								"timeout": "tomorrow",
							},
						},
					},
				},
			},
			err: "pipeline is not compliant with schema pipeline.schema.v1",
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

func TestARMStackSchemaValidation(t *testing.T) {
	const schemaErr = "pipeline is not compliant with schema pipeline.schema.v1"
	createStep := func() map[string]interface{} {
		return map[string]interface{}{
			"name":             "deploy",
			"action":           "ARMStack",
			"template":         "templates/main.bicep",
			"parameters":       "configurations/main.tmpl.bicepparam",
			"deploymentLevel":  "ResourceGroup",
			"actionOnUnmanage": "delete",
		}
	}
	deleteStep := func() map[string]interface{} {
		return map[string]interface{}{
			"name":                         "remove-acrs-stack",
			"action":                       "ARMStack",
			"operation":                    "delete",
			"deploymentLevel":              "ResourceGroup",
			"deploymentStackNameForDelete": map[string]interface{}{"value": "Global.singleton.acrs"},
			"actionOnUnmanage":             "detach",
			"identityFrom": map[string]interface{}{
				"resourceGroup": "global",
				"step":          "output",
				"name":          "globalMSIId",
			},
		}
	}

	for _, tc := range []struct {
		name   string
		step   func() map[string]interface{}
		mutate func(step map[string]interface{})
		err    string
	}{
		{name: "create without operation", step: createStep},
		{name: "create with explicit operation", step: createStep, mutate: func(s map[string]interface{}) { s["operation"] = "createOrUpdate" }},
		{name: "create missing template", step: createStep, mutate: func(s map[string]interface{}) { delete(s, "template") }, err: schemaErr},
		{name: "create missing parameters", step: createStep, mutate: func(s map[string]interface{}) { delete(s, "parameters") }, err: schemaErr},
		{name: "create missing actionOnUnmanage", step: createStep, mutate: func(s map[string]interface{}) { delete(s, "actionOnUnmanage") }, err: schemaErr},
		{name: "create rejects deploymentStackNameForDelete", step: createStep, mutate: func(s map[string]interface{}) {
			s["deploymentStackNameForDelete"] = map[string]interface{}{"value": "Global.singleton.acrs"}
		}, err: schemaErr},
		{name: "create rejects identityFrom", step: createStep, mutate: func(s map[string]interface{}) {
			s["identityFrom"] = map[string]interface{}{"resourceGroup": "global", "step": "output", "name": "globalMSIId"}
		}, err: schemaErr},
		{name: "create rejects timeout", step: createStep, mutate: func(s map[string]interface{}) { s["timeout"] = "30m" }, err: schemaErr},
		{name: "unknown operation", step: createStep, mutate: func(s map[string]interface{}) { s["operation"] = "destroy" }, err: schemaErr},

		{name: "delete with detach", step: deleteStep},
		{name: "delete with delete", step: deleteStep, mutate: func(s map[string]interface{}) { s["actionOnUnmanage"] = "delete" }},
		{name: "delete with timeout", step: deleteStep, mutate: func(s map[string]interface{}) { s["timeout"] = "30m" }},
		{name: "delete with name from config", step: deleteStep, mutate: func(s map[string]interface{}) {
			s["deploymentStackNameForDelete"] = map[string]interface{}{"configRef": "global.acrStackName"}
		}},
		{name: "delete with name from input", step: deleteStep, mutate: func(s map[string]interface{}) {
			s["deploymentStackNameForDelete"] = map[string]interface{}{"input": map[string]interface{}{"resourceGroup": "global", "step": "output", "name": "stackName"}}
		}},
		{name: "delete missing deploymentStackNameForDelete", step: deleteStep, mutate: func(s map[string]interface{}) { delete(s, "deploymentStackNameForDelete") }, err: schemaErr},
		{name: "delete with bare string deploymentStackNameForDelete", step: deleteStep, mutate: func(s map[string]interface{}) { s["deploymentStackNameForDelete"] = "Global.singleton.acrs" }, err: schemaErr},
		{name: "delete missing actionOnUnmanage", step: deleteStep, mutate: func(s map[string]interface{}) { delete(s, "actionOnUnmanage") }, err: schemaErr},
		{name: "delete missing identityFrom", step: deleteStep, mutate: func(s map[string]interface{}) { delete(s, "identityFrom") }, err: schemaErr},
		{name: "delete invalid timeout", step: deleteStep, mutate: func(s map[string]interface{}) { s["timeout"] = "soon" }, err: schemaErr},
		{name: "delete at subscription level", step: deleteStep, mutate: func(s map[string]interface{}) { s["deploymentLevel"] = "Subscription" }, err: schemaErr},
		{name: "delete rejects template", step: deleteStep, mutate: func(s map[string]interface{}) { s["template"] = "templates/main.bicep" }, err: schemaErr},
		{name: "delete rejects parameters", step: deleteStep, mutate: func(s map[string]interface{}) { s["parameters"] = "main.bicepparam" }, err: schemaErr},
		{name: "delete rejects variables", step: deleteStep, mutate: func(s map[string]interface{}) {
			s["variables"] = []interface{}{map[string]interface{}{"name": "x", "value": "y"}}
		}, err: schemaErr},
		{name: "delete rejects bypassStackOutOfSyncError", step: deleteStep, mutate: func(s map[string]interface{}) { s["bypassStackOutOfSyncError"] = true }, err: schemaErr},
	} {
		t.Run(tc.name, func(t *testing.T) {
			step := tc.step()
			if tc.mutate != nil {
				tc.mutate(step)
			}
			pipeline := map[string]interface{}{
				"serviceGroup": "test",
				"rolloutName":  "test",
				"resourceGroups": []interface{}{
					map[string]interface{}{
						"name":          "rg",
						"resourceGroup": "rg",
						"subscription":  "sub",
						"steps":         []interface{}{step},
					},
				},
			}
			pipelineBytes, err := yaml.Marshal(pipeline)
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

func TestGrafanaADXIntegrationsSchemaValidation(t *testing.T) {
	newPipeline := func(adx map[string]interface{}) map[string]interface{} {
		return map[string]interface{}{
			"serviceGroup": "test",
			"rolloutName":  "test",
			"resourceGroups": []interface{}{
				map[string]interface{}{
					"name":          "rg",
					"resourceGroup": "rg",
					"subscription":  "sub",
					"steps": []interface{}{
						map[string]interface{}{
							"name":        "grafana",
							"action":      "GrafanaManage",
							"grafanaName": map[string]interface{}{"value": "grafana"},
							"location":    map[string]interface{}{"value": "eastus"},
							"adx":         adx,
							"identityFrom": map[string]interface{}{
								"resourceGroup": "rg",
								"step":          "identity",
								"name":          "resourceId",
							},
						},
					},
				},
			},
		}
	}
	validate := func(t *testing.T, pipeline map[string]interface{}) error {
		t.Helper()
		content, err := json.Marshal(pipeline)
		assert.NilError(t, err)
		return ValidatePipelineSchema(content)
	}

	t.Run("valid", func(t *testing.T) {
		err := validate(t, newPipeline(map[string]interface{}{
			"enabled":          map[string]interface{}{"value": "true"},
			"environment":      map[string]interface{}{"value": "int"},
			"geographies":      map[string]interface{}{"value": "eus,wus"},
			"scenario":         map[string]interface{}{"configRef": "grafana.adx.scenario"},
			"targetResourceId": map[string]interface{}{"configRef": "grafana.adx.targetResourceId"},
		}))
		assert.NilError(t, err)
	})

	t.Run("enabled required", func(t *testing.T) {
		err := validate(t, newPipeline(map[string]interface{}{
			"geographies": map[string]interface{}{"value": "eus"},
		}))
		assert.ErrorContains(t, err, "pipeline is not compliant with schema")
	})

	t.Run("unknown field rejected", func(t *testing.T) {
		err := validate(t, newPipeline(map[string]interface{}{
			"enabled": map[string]interface{}{"value": "true"},
			"unknown": map[string]interface{}{"value": "value"},
		}))
		assert.ErrorContains(t, err, "pipeline is not compliant with schema")
	})
}
