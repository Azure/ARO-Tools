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

	"github.com/stretchr/testify/assert"

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
	assert.NoError(t, err)

	configProvider, err := config.NewConfigProvider("../../testdata/config.yaml")
	assert.NoError(t, err)
	configResolver, err := configProvider.GetResolver(&config.ConfigReplacements{
		RegionReplacement:      region,
		RegionShortReplacement: regionShort,
		StampReplacement:       stamp,
		CloudReplacement:       cloud,
		EnvironmentReplacement: environment,
		Ev2Config:              ev2,
	})
	assert.NoError(t, err)

	cfg, err := configResolver.GetRegionConfiguration(region)
	assert.NoError(t, err)
	assert.NotNil(t, cfg)

	testutil.CompareWithFixture(t, cfg)
}

func TestConfigProviderTypeConsistency(t *testing.T) {
	testCases := []struct {
		name                   string
		getConfigFunc          func(resolver config.ConfigResolver) (types.Configuration, error)
		expectedSpecificFields []string // specific fields we expect to exist as Configuration types
	}{
		{
			name: "cloud and environment configuration",
			getConfigFunc: func(resolver config.ConfigResolver) (types.Configuration, error) {
				return resolver.GetConfiguration()
			},
			expectedSpecificFields: []string{"clustersService"},
		},
		{
			name: "region-specific configuration",
			getConfigFunc: func(resolver config.ConfigResolver) (types.Configuration, error) {
				return resolver.GetRegionConfiguration("uksouth")
			},
			expectedSpecificFields: []string{"svc", "geneva"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Define inline helper function
			var verifyNoMapStringInterface func(path string, v interface{}, inconsistentPaths *[]string)
			verifyNoMapStringInterface = func(path string, v interface{}, inconsistentPaths *[]string) {
				switch val := v.(type) {
				case types.Configuration:
					for k, nested := range val {
						verifyNoMapStringInterface(path+"."+k, nested, inconsistentPaths)
					}
				case map[string]interface{}:
					*inconsistentPaths = append(*inconsistentPaths, path)
					for k, nested := range val {
						verifyNoMapStringInterface(path+"."+k, nested, inconsistentPaths)
					}
				}
			}

			configProvider, err := config.NewConfigProvider("../../testdata/config.yaml")
			assert.NoError(t, err)

			resolver, err := configProvider.GetResolver(&config.ConfigReplacements{
				CloudReplacement:       "public",
				EnvironmentReplacement: "int",
				RegionReplacement:      "uksouth",
				RegionShortReplacement: "uks",
				StampReplacement:       "1",
				Ev2Config: map[string]interface{}{
					"keyVault": map[string]interface{}{
						"domainNameSuffix": ".vault.azure.net",
					},
					"availabilityZoneCount": 3,
				},
			})
			assert.NoError(t, err)

			cfg, err := tc.getConfigFunc(resolver)
			assert.NoError(t, err)

			// Comprehensive type consistency check using shared helper
			inconsistentPaths := []string{}
			verifyNoMapStringInterface("root", cfg, &inconsistentPaths)
			assert.Empty(t, inconsistentPaths, "Found map[string]interface{} at paths: %v (should be types.Configuration)", inconsistentPaths)

			// Verify that expected fields exist (the validator already checked their types)
			for _, fieldName := range tc.expectedSpecificFields {
				assert.Contains(t, cfg, fieldName, "Expected field %s should exist in configuration", fieldName)
			}
		})
	}
}

func TestInterfaceToConfiguration(t *testing.T) {
	testCases := []struct {
		name                   string
		i                      interface{}
		ok                     bool
		expecetedConfiguration types.Configuration
	}{
		{
			name:                   "empty interface",
			ok:                     false,
			expecetedConfiguration: types.Configuration{},
		},
		{
			name:                   "empty map",
			i:                      map[string]interface{}{},
			ok:                     true,
			expecetedConfiguration: types.Configuration{},
		},
		{
			name: "map",
			i: map[string]interface{}{
				"key1": "value1",
				"key2": "value2",
			},
			ok: true,
			expecetedConfiguration: types.Configuration{
				"key1": "value1",
				"key2": "value2",
			},
		},
		{
			name: "nested map",
			i: map[string]interface{}{
				"key1": map[string]interface{}{
					"key2": "value2",
				},
			},
			ok: true,
			expecetedConfiguration: types.Configuration{
				"key1": types.Configuration{
					"key2": "value2",
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			vars, ok := types.InterfaceToConfiguration(tc.i)
			assert.Equal(t, tc.ok, ok)
			assert.Equal(t, tc.expecetedConfiguration, vars)
		})
	}
}

func TestMergeConfiguration(t *testing.T) {
	testCases := []struct {
		name     string
		base     types.Configuration
		override types.Configuration
		expected types.Configuration
	}{
		{
			name:     "nil base",
			expected: types.Configuration{},
		},
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
			base:     types.Configuration{"key1": types.Configuration{"key2": "value2"}},
			override: types.Configuration{"key1": "value1"},
			expected: types.Configuration{"key1": "value1"},
		},
		{
			name:     "merge into sub map",
			base:     types.Configuration{"key1": types.Configuration{"key2": "value2"}},
			override: types.Configuration{"key1": types.Configuration{"key3": "value3"}},
			expected: types.Configuration{"key1": types.Configuration{"key2": "value2", "key3": "value3"}},
		},
		{
			name:     "override sub map value",
			base:     types.Configuration{"key1": types.Configuration{"key2": "value2"}},
			override: types.Configuration{"key1": types.Configuration{"key2": "value3"}},
			expected: types.Configuration{"key1": types.Configuration{"key2": "value3"}},
		},
		{
			name:     "override nested sub map",
			base:     types.Configuration{"key1": types.Configuration{"key2": types.Configuration{"key3": "value3"}}},
			override: types.Configuration{"key1": types.Configuration{"key2": types.Configuration{"key3": "value4"}}},
			expected: types.Configuration{"key1": types.Configuration{"key2": types.Configuration{"key3": "value4"}}},
		},
		{
			name:     "override nested sub map multiple levels",
			base:     types.Configuration{"key1": types.Configuration{"key2": types.Configuration{"key3": "value3"}}},
			override: types.Configuration{"key1": types.Configuration{"key2": types.Configuration{"key4": "value4"}}, "key5": "value5"},
			expected: types.Configuration{"key1": types.Configuration{"key2": types.Configuration{"key3": "value3", "key4": "value4"}}, "key5": "value5"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := types.MergeConfiguration(tc.base, tc.override)
			assert.Equal(t, tc.expected, result)
		})
	}

}

func TestPreprocessContent(t *testing.T) {
	fileContent, err := os.ReadFile("../../testdata/test.bicepparam")
	assert.Nil(t, err)

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
	assert.Nil(t, err)
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
				assert.NotNil(t, err)
			} else {
				assert.Nil(t, err)
			}
		})
	}
}
