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
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"gopkg.in/yaml.v3"

	"github.com/Azure/ARO-Tools/internal/testutil"
)

func TestNewPlainPipelineFromBytes(t *testing.T) {
	pipelineBytes, err := os.ReadFile("../../testdata/zz_fixture_TestNewPlainPipelineFromBytes.yaml")
	assert.NoError(t, err)

	p, err := NewPlainPipelineFromBytes("", pipelineBytes)
	assert.NoError(t, err)

	pipelineBytes, err = yaml.Marshal(p)
	assert.NoError(t, err)

	testutil.CompareWithFixture(t, pipelineBytes, testutil.WithExtension(".yaml"))

}

func TestUnmarshalStep(t *testing.T) {
	tests := []struct {
		name     string
		rawStep  map[string]interface{}
		expected Step
	}{
		{
			name: "Test Shell Step",
			rawStep: map[string]interface{}{
				"action":  "Shell",
				"name":    "test",
				"command": "bash",
			},
			expected: NewShellStep("test", "bash"),
		},
		{
			name: "Test ARM Step",
			rawStep: map[string]interface{}{
				"action":          "ARM",
				"name":            "test",
				"template":        "foo.bicep",
				"parameters":      "foo.bicepparam",
				"deploymentLevel": "Subscription",
			},
			expected: NewARMStep("test", "foo.bicep", "foo.bicepparam", "Subscription"),
		},
		{
			name: "Test DNS Step",
			rawStep: map[string]interface{}{
				"action": "DelegateChildZone",
				"name":   "test",
			},
			expected: NewDNSStep("test"),
		},
		{
			name: "Test RP Step",
			rawStep: map[string]interface{}{
				"action": "ResourceProviderRegistration",
				"name":   "test",
			},
			expected: NewRPStep("test"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			step, err := unmarshalStep(tt.rawStep)
			assert.NoError(t, err)
			assert.Equal(t, tt.expected, step)
		})
	}
}
