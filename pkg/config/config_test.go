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
	"fmt"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/Azure/ARO-Tools/internal/testutil"
	"github.com/Azure/ARO-Tools/pkg/config"
	"github.com/Azure/ARO-Tools/pkg/config/ev2config"
)

func TestConfigProvider(t *testing.T) {
	region := "uksouth"
	regionShort := "uks"
	stamp := "1"
	cloud := "public"
	environment := "int"

	configProvider := config.NewConfigProvider("../../testdata/config.yaml")
	ev2, err := ev2config.Config()
	assert.NoError(t, err)

	cfg, err := configProvider.GetDeployEnvRegionConfiguration(cloud, environment, region, &config.ConfigReplacements{
		RegionReplacement:      region,
		RegionShortReplacement: regionShort,
		StampReplacement:       stamp,
		CloudReplacement:       cloud,
		EnvironmentReplacement: environment,
		Ev2Config:              ev2.ResolveRegion(cloud, "prod", region),
	})
	assert.NoError(t, err)
	assert.NotNil(t, cfg)

	// key is not in the config file
	assert.Nil(t, cfg["svc_resourcegroup"])

	// key is in the config file, region constant value
	assert.Equal(t, "uksouth", cfg["test"])

	// key is in the config file, default in INT, constant value
	assert.Equal(t, "aro-hcp-int.azurecr.io/maestro-server:the-stable-one", cfg["maestro_image"])

	// key is in the config file, default, varaible value
	assert.Equal(t, fmt.Sprintf("hcp-underlay-%s", regionShort), cfg["regionRG"])

	// key is in the config file, varaible value
	assert.Equal(t, fmt.Sprintf("%s-%s", cloud, environment), cfg["cloudEnv"])
}

func TestInterfaceToConfiguration(t *testing.T) {
	testCases := []struct {
		name                   string
		i                      interface{}
		ok                     bool
		expecetedConfiguration config.Configuration
	}{
		{
			name:                   "empty interface",
			ok:                     false,
			expecetedConfiguration: config.Configuration{},
		},
		{
			name:                   "empty map",
			i:                      map[string]interface{}{},
			ok:                     true,
			expecetedConfiguration: config.Configuration{},
		},
		{
			name: "map",
			i: map[string]interface{}{
				"key1": "value1",
				"key2": "value2",
			},
			ok: true,
			expecetedConfiguration: config.Configuration{
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
			expecetedConfiguration: config.Configuration{
				"key1": config.Configuration{
					"key2": "value2",
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			vars, ok := config.InterfaceToConfiguration(tc.i)
			assert.Equal(t, tc.ok, ok)
			assert.Equal(t, tc.expecetedConfiguration, vars)
		})
	}
}

func TestMergeConfiguration(t *testing.T) {
	testCases := []struct {
		name     string
		base     config.Configuration
		override config.Configuration
		expected config.Configuration
	}{
		{
			name:     "nil base",
			expected: nil,
		},
		{
			name:     "empty base and override",
			base:     config.Configuration{},
			expected: config.Configuration{},
		},
		{
			name:     "merge into empty base",
			base:     config.Configuration{},
			override: config.Configuration{"key1": "value1"},
			expected: config.Configuration{"key1": "value1"},
		},
		{
			name:     "merge into base",
			base:     config.Configuration{"key1": "value1"},
			override: config.Configuration{"key2": "value2"},
			expected: config.Configuration{"key1": "value1", "key2": "value2"},
		},
		{
			name:     "override base, change schema",
			base:     config.Configuration{"key1": config.Configuration{"key2": "value2"}},
			override: config.Configuration{"key1": "value1"},
			expected: config.Configuration{"key1": "value1"},
		},
		{
			name:     "merge into sub map",
			base:     config.Configuration{"key1": config.Configuration{"key2": "value2"}},
			override: config.Configuration{"key1": config.Configuration{"key3": "value3"}},
			expected: config.Configuration{"key1": config.Configuration{"key2": "value2", "key3": "value3"}},
		},
		{
			name:     "override sub map value",
			base:     config.Configuration{"key1": config.Configuration{"key2": "value2"}},
			override: config.Configuration{"key1": config.Configuration{"key2": "value3"}},
			expected: config.Configuration{"key1": config.Configuration{"key2": "value3"}},
		},
		{
			name:     "override nested sub map",
			base:     config.Configuration{"key1": config.Configuration{"key2": config.Configuration{"key3": "value3"}}},
			override: config.Configuration{"key1": config.Configuration{"key2": config.Configuration{"key3": "value4"}}},
			expected: config.Configuration{"key1": config.Configuration{"key2": config.Configuration{"key3": "value4"}}},
		},
		{
			name:     "override nested sub map multiple levels",
			base:     config.Configuration{"key1": config.Configuration{"key2": config.Configuration{"key3": "value3"}}},
			override: config.Configuration{"key1": config.Configuration{"key2": config.Configuration{"key4": "value4"}}, "key5": "value5"},
			expected: config.Configuration{"key1": config.Configuration{"key2": config.Configuration{"key3": "value3", "key4": "value4"}}, "key5": "value5"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := config.MergeConfiguration(tc.base, tc.override)
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
