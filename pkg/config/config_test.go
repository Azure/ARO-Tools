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
	"os"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/require"

	"github.com/Azure/ARO-Tools/internal/testutil"
	"github.com/Azure/ARO-Tools/pkg/config"
	"github.com/Azure/ARO-Tools/pkg/config/ev2config"
	"github.com/Azure/ARO-Tools/pkg/config/types"
)

func TestConfigProvider(t *testing.T) {
	region := "uksouth"
	regionShort := "uks"
	stamp := "1"
	cloud := "public"
	environment := "int"

	ev2, err := ev2config.ResolveConfig(cloud, region)
	require.NoError(t, err)

	configProvider, err := config.NewConfigProvider("../../testdata/config.yaml")
	require.NoError(t, err)
	configResolver, err := configProvider.GetResolver(&config.ConfigReplacements{
		RegionReplacement:      region,
		RegionShortReplacement: regionShort,
		StampReplacement:       stamp,
		CloudReplacement:       cloud,
		EnvironmentReplacement: environment,
		Ev2Config:              ev2,
	})
	require.NoError(t, err)

	cfg, err := configResolver.GetRegionConfiguration(region)
	require.NoError(t, err)
	require.NotNil(t, cfg)

	testutil.CompareWithFixture(t, cfg)
}

func TestMergeConfiguration(t *testing.T) {
	testCases := []struct {
		name     string
		base     types.Configuration
		override types.Configuration
		expected types.Configuration
	}{
		{
			name:     "empty base and override",
			base:     types.Configuration{},
			expected: types.Configuration{},
		},
		{
			name:     "merge into empty base",
			base:     types.Configuration{},
			override: types.Configuration{"key1": "value1"},
			expected: types.Configuration{"key1": "value1"},
		},
		{
			name:     "merge into base",
			base:     types.Configuration{"key1": "value1"},
			override: types.Configuration{"key2": "value2"},
			expected: types.Configuration{"key1": "value1", "key2": "value2"},
		},
		{
			name:     "override base, change schema",
			base:     types.Configuration{"key1": map[string]any{"key2": "value2"}},
			override: types.Configuration{"key1": "value1"},
			expected: types.Configuration{"key1": "value1"},
		},
		{
			name:     "merge into sub map",
			base:     types.Configuration{"key1": map[string]any{"key2": "value2"}},
			override: types.Configuration{"key1": map[string]any{"key3": "value3"}},
			expected: types.Configuration{"key1": map[string]any{"key2": "value2", "key3": "value3"}},
		},
		{
			name:     "override sub map value",
			base:     types.Configuration{"key1": map[string]any{"key2": "value2"}},
			override: types.Configuration{"key1": map[string]any{"key2": "value3"}},
			expected: types.Configuration{"key1": map[string]any{"key2": "value3"}},
		},
		{
			name:     "override nested sub map",
			base:     types.Configuration{"key1": map[string]any{"key2": map[string]any{"key3": "value3"}}},
			override: types.Configuration{"key1": map[string]any{"key2": map[string]any{"key3": "value4"}}},
			expected: types.Configuration{"key1": map[string]any{"key2": map[string]any{"key3": "value4"}}},
		},
		{
			name:     "override nested sub map multiple levels",
			base:     types.Configuration{"key1": map[string]any{"key2": map[string]any{"key3": "value3"}}},
			override: types.Configuration{"key1": map[string]any{"key2": map[string]any{"key4": "value4"}}, "key5": "value5"},
			expected: types.Configuration{"key1": map[string]any{"key2": map[string]any{"key3": "value3", "key4": "value4"}}, "key5": "value5"},
		},
		{
			name:     "non-string-key maps get overridden, not merged",
			base:     types.Configuration{"key1": map[int]any{1: map[string]any{"key3": "value3"}}},
			override: types.Configuration{"key1": map[int]any{2: map[string]any{"key4": "value4"}}, "key5": "value5"},
			expected: types.Configuration{"key1": map[int]any{2: map[string]any{"key4": "value4"}}, "key5": "value5"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			types.MergeConfiguration(tc.base, tc.override)
			require.Empty(t, cmp.Diff(tc.expected, tc.base))
		})
	}

}

func TestPreprocessContent(t *testing.T) {
	fileContent, err := os.ReadFile("../../testdata/test.bicepparam")
	require.Nil(t, err)

	processed, err := config.PreprocessContent(
		fileContent,
		map[string]any{
			"regionRG": "bahamas",
			"clustersService": map[string]any{
				"imageTag": "cs-image",
			},
			"availabilityZoneCount": 3,
		},
	)
	require.Nil(t, err)
	testutil.CompareWithFixture(t, processed, testutil.WithExtension(".bicepparam"))
}

func TestPreprocessContentMissingKey(t *testing.T) {
	testCases := []struct {
		name       string
		content    string
		vars       map[string]any
		shouldFail bool
	}{
		{
			name:    "missing key",
			content: "foo: {{ .bar }}",
			vars: map[string]any{
				"baz": "bar",
			},
			shouldFail: true,
		},
		{
			name:    "missing nested key",
			content: "foo: {{ .bar.baz }}",
			vars: map[string]any{
				"baz": "bar",
			},
			shouldFail: true,
		},
		{
			name:    "no missing key",
			content: "foo: {{ .bar }}",
			vars: map[string]any{
				"bar": "bar",
			},
			shouldFail: false,
		},
		{
			name:    "no missing nested key",
			content: "foo: {{ .bar.baz }}",
			vars: map[string]any{
				"bar": map[string]any{
					"baz": "baz",
				},
			},
			shouldFail: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := config.PreprocessContent(
				[]byte(tc.content),
				tc.vars,
			)
			if tc.shouldFail {
				require.NotNil(t, err)
			} else {
				require.Nil(t, err)
			}
		})
	}
}
