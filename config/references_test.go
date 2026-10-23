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

package config_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"k8s.io/apimachinery/pkg/util/sets"

	"github.com/Azure/ARO-Tools/config"
	"github.com/Azure/ARO-Tools/config/types"
	"github.com/Azure/ARO-Tools/testutil"
)

func TestDunderRendering(t *testing.T) {
	configuration := types.Configuration{
		"acr": map[string]any{
			"name":  "registry",
			"alias": "registry",
		},
		"enabled":  false,
		"replicas": 3,
		"regions":  []any{"eastus", "westus"},
		"optional": nil,
	}
	content := []byte(`registry: {{ .acr.name }}
alias: {{ .acr.alias }}
enabled: {{ .enabled }}
replicas: {{ .replicas }}
regions: {{ .regions }}
optional: {{ .optional }}
input: __clientId__
`)
	original, err := config.PreprocessContent(content, configuration)
	require.NoError(t, err)

	_, dunderCfg := config.EV2Mapping(configuration, config.NewDunderPlaceholders(), nil)
	rendered, references, err := config.PrepareEv2Template(content, dunderCfg)
	require.NoError(t, err)
	testutil.CompareWithFixture(t, rendered)
	require.Equal(t, sets.New("acr.name", "acr.alias", "enabled", "replicas", "regions", "optional", "clientId"), references)

	// Constructing the placeholder configuration must not change later real renders.
	after, err := config.PreprocessContent(content, configuration)
	require.NoError(t, err)
	require.Equal(t, original, after)
}

func TestExtractDunderReferences(t *testing.T) {
	for _, tt := range []struct {
		name    string
		content string
		want    []string
	}{
		{
			name:    "repeated names share one parameter",
			content: "__acr.svc.name__ __acr.svc.name__ __clientId__ __clientId__",
			want:    []string{"acr.svc.name", "clientId"},
		},
		{
			name:    "adjacent placeholders remain distinct and case sensitive",
			content: "__Name1____name1__",
			want:    []string{"Name1", "name1"},
		},
		{
			name:    "only complete placeholders in the supported alphabet match",
			content: "__bad_name__ __bad-name__ __ümlaut__ __ __unfinished",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, sets.New(tt.want...), config.ExtractDunderReferences([]byte(tt.content)))
		})
	}
}
