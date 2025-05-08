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

package ev2

import (
	"path/filepath"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"gotest.tools/assert"

	"github.com/Azure/ARO-Tools/pkg/config"
	"github.com/Azure/ARO-Tools/pkg/types"
)

func TestScopeBindingVariables(t *testing.T) {
	configProvider := config.NewConfigProvider("../../testdata/config.yaml")
	vars, err := ScopeBindingVariables(configProvider, "public", "int")
	if err != nil {
		t.Fatalf("ScopeBindingVariables failed: %v", err)
	}
	expectedVars := map[string]string{
		"__aksName__":                       "$config(aksName)",
		"__childZone__":                     "$config(childZone)",
		"__globalRG__":                      "$config(globalRG)",
		"__imageSyncRG__":                   "$config(imageSyncRG)",
		"__maestro_helm_chart__":            "$config(maestro_helm_chart)",
		"__maestro_image__":                 "$config(maestro_image)",
		"__managementClusterRG__":           "$config(managementClusterRG)",
		"__managementClusterSubscription__": "$config(managementClusterSubscription)",
		"__parentZone__":                    "$config(parentZone)",
		"__provider__":                      "$config(provider)",
		"__region__":                        "$config(region)",
		"__regionRG__":                      "$config(regionRG)",
		"__serviceClusterRG__":              "$config(serviceClusterRG)",
		"__serviceClusterSubscription__":    "$config(serviceClusterSubscription)",
		"__vaultBaseUrl__":                  "$config(vaultBaseUrl)",
		"__clustersService.imageTag__":      "$config(clustersService.imageTag)",
		"__clustersService.replicas__":      "$config(clustersService.replicas)",
		"__enableOptionalStep__":            "$config(enableOptionalStep)",
	}

	if diff := cmp.Diff(expectedVars, vars); diff != "" {
		t.Errorf("got incorrect vars: %v", diff)
	}
}

func TestDeepCopy(t *testing.T) {
	configProvider := config.NewConfigProvider("../../testdata/config.yaml")
	vars, err := configProvider.GetDeployEnvRegionConfiguration("public", "int", "", config.NewConfigReplacements("r", "sr", "s"))
	if err != nil {
		t.Errorf("failed to get variables: %v", err)
	}
	pipeline, err := types.NewPipelineFromFile("../../testdata/pipeline.yaml", vars)
	if err != nil {
		t.Errorf("failed to read new pipeline: %v", err)
	}

	newPipelinePath := "new-pipeline.yaml"
	pipelineCopy, err := deepCopyPipeline(pipeline, newPipelinePath)
	if err != nil {
		t.Errorf("failed to copy pipeline: %v", err)
	}

	assert.Assert(t, pipeline != pipelineCopy, "expected pipeline and copy to be different")

	if diff := cmp.Diff(pipeline, pipelineCopy, cmpopts.IgnoreUnexported(types.Pipeline{}, types.ShellStep{}, types.ARMStep{})); diff != "" {
		t.Errorf("got diffs after pipeline deep copy: %v", diff)
	}
}

func TestAbsoluteFilePath(t *testing.T) {
	pipelineFilePath := "../../testdata/pipeline.yaml"

	abspath := func(path string) string {
		abs, _ := filepath.Abs(path)
		return abs
	}
	testCases := []struct {
		name         string
		relativeFile string
		absoluteFile string
	}{
		{
			name:         "basic",
			relativeFile: "test.bicepparam",
			absoluteFile: abspath("../../testdata/test.bicepparam"),
		},
		{
			name:         "go one lower",
			relativeFile: "../test.bicepparam",
			absoluteFile: abspath("../../test.bicepparam"),
		},
		{
			name:         "subdir",
			relativeFile: "subdir/test.bicepparam",
			absoluteFile: abspath("../../testdata/subdir/test.bicepparam"),
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			abs, err := absoluteFilePath(pipelineFilePath, tc.relativeFile)
			if err != nil {
				t.Errorf("failed to get absolute file path: %v", err)
			}
			assert.Equal(t, abs, tc.absoluteFile, "expected absolute file path to be correct")
		})
	}
}
