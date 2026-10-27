// Copyright 2026 Microsoft Corporation
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

	"github.com/stretchr/testify/require"
)

func TestIstioBootstrapDiscoverySchemaAndDeserialization(t *testing.T) {
	for _, tt := range []struct {
		name    string
		mutate  func(map[string]any)
		invalid bool
	}{
		{name: "configured target and versions"},
		{name: "optional identity", mutate: func(step map[string]any) {
			step["identityFrom"] = map[string]any{"resourceGroup": "global", "step": "identity", "name": "resourceId"}
		}},
		{name: "input-bound values", mutate: func(step map[string]any) {
			step["aksCluster"] = map[string]any{"input": map[string]any{"resourceGroup": "rg", "step": "target", "name": "clusterName"}}
			step["versions"] = map[string]any{"input": map[string]any{"resourceGroup": "rg", "step": "config", "name": "versions"}}
		}},
		{name: "missing target", invalid: true, mutate: func(step map[string]any) { delete(step, "aksCluster") }},
		{name: "missing versions", invalid: true, mutate: func(step map[string]any) { delete(step, "versions") }},
		{name: "bare versions not a Value", invalid: true, mutate: func(step map[string]any) { step["versions"] = []string{"asm-1-29"} }},
		{name: "unknown property", invalid: true, mutate: func(step map[string]any) { step["kubeconfig"] = "unneeded" }},
		{name: "invalid identity", invalid: true, mutate: func(step map[string]any) { step["identityFrom"] = "identity" }},
	} {
		t.Run(tt.name, func(t *testing.T) {
			step := map[string]any{
				"name": "bootstrap", "action": StepActionIstioBootstrapDiscovery,
				"aksCluster": map[string]any{"configRef": "svc.aks.name"},
				"versions":   map[string]any{"configRef": "svc.istio.versions"},
			}
			if tt.mutate != nil {
				tt.mutate(step)
			}
			data, err := json.Marshal(map[string]any{
				"serviceGroup": "test", "rolloutName": "test",
				"resourceGroups": []any{map[string]any{
					"name": "rg", "resourceGroup": "rg", "subscription": "sub", "steps": []any{step},
				}},
			})
			require.NoError(t, err)
			err = ValidatePipelineSchema(data)
			if tt.invalid {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			var pipeline Pipeline
			require.NoError(t, json.Unmarshal(data, &pipeline))
			_, ok := pipeline.ResourceGroups[0].Steps[0].(*IstioBootstrapDiscoveryStep)
			require.True(t, ok, "native discovery must not deserialize as GenericStep")
		})
	}
}

func TestIstioBootstrapDiscoveryRequiredInputs(t *testing.T) {
	cluster := StepDependency{ResourceGroup: "rg", Step: "cluster"}
	versions := StepDependency{ResourceGroup: "rg", Step: "versions"}
	identity := StepDependency{ResourceGroup: "global", Step: "identity"}
	step := &IstioBootstrapDiscoveryStep{
		AKSCluster:   Value{Input: &Input{StepDependency: cluster, Name: "name"}},
		Versions:     Value{Input: &Input{StepDependency: versions, Name: "versions"}},
		IdentityFrom: Input{StepDependency: identity, Name: "resourceId"},
	}
	require.Equal(t, []StepDependency{identity, cluster, versions}, step.RequiredInputs())
	step.Versions.Input.StepDependency = cluster
	require.Equal(t, []StepDependency{identity, cluster}, step.RequiredInputs(), "shared input steps should be deduplicated")
	require.Empty(t, (&IstioBootstrapDiscoveryStep{}).RequiredInputs())
	require.False(t, step.IsWellFormedOverInputs(), "live ARM state must not leak through cross-execution caches")
}
