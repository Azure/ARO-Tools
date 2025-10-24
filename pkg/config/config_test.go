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

func TestConfigProvenance(t *testing.T) {
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

	ubiquitous, err := configResolver.ValueProvenance(region, "ubiquitousValue")
	require.NoError(t, err)

	if diff := cmp.Diff(ubiquitous, &config.Provenance{
		Default:        "global-value",
		DefaultSet:     true,
		Cloud:          "public-value",
		CloudSet:       true,
		Environment:    "public-int-value",
		EnvironmentSet: true,
		Region:         "public-int-uksouth-value",
		RegionSet:      true,
		Result:         "public-int-uksouth-value",
		ResultSet:      true,
	}); diff != "" {
		t.Errorf("Provenance mismatch for ubiquitousValue (-want +got):\n%s", diff)
	}

	partial, err := configResolver.ValueProvenance(region, "partialValue")
	require.NoError(t, err)

	if diff := cmp.Diff(partial, &config.Provenance{
		Default:        "global-value",
		DefaultSet:     true,
		Cloud:          nil,
		CloudSet:       false,
		Environment:    nil,
		EnvironmentSet: false,
		Region:         "public-int-uksouth-value",
		RegionSet:      true,
		Result:         "public-int-uksouth-value",
		ResultSet:      true,
	}); diff != "" {
		t.Errorf("Provenance mismatch for partialValue (-want +got):\n%s", diff)
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
			output := types.Configuration(types.MergeConfiguration(tc.base, tc.override))
			require.Empty(t, cmp.Diff(tc.expected, output))
		})
	}

}

func TestTruncateConfiguration(t *testing.T) {
	testCases := []struct {
		name        string
		config      types.Configuration
		paths       []string
		expected    map[string]any
		expectError bool
		errorMsg    string
	}{
		// Error cases
		{
			name:        "nil config",
			config:      nil,
			paths:       []string{"key1"},
			expected:    nil,
			expectError: true,
			errorMsg:    "config cannot be nil",
		},
		{
			name:        "empty paths",
			config:      types.Configuration{"key1": "value1"},
			paths:       []string{},
			expected:    nil,
			expectError: true,
			errorMsg:    "no paths provided for truncation",
		},
		{
			name:        "invalid path - empty string",
			config:      types.Configuration{"key1": "value1"},
			paths:       []string{""},
			expected:    nil,
			expectError: true,
			errorMsg:    "invalid truncate path",
		},
		{
			name:        "invalid path - starts with dot",
			config:      types.Configuration{"key1": "value1"},
			paths:       []string{".key1"},
			expected:    nil,
			expectError: true,
			errorMsg:    "invalid truncate path",
		},
		{
			name:        "invalid path - ends with dot",
			config:      types.Configuration{"key1": "value1"},
			paths:       []string{"key1."},
			expected:    nil,
			expectError: true,
			errorMsg:    "invalid truncate path",
		},
		{
			name:        "invalid path - consecutive dots",
			config:      types.Configuration{"key1": "value1"},
			paths:       []string{"key1..key2"},
			expected:    nil,
			expectError: true,
			errorMsg:    "invalid truncate path",
		},
		{
			name:        "invalid path - just a dot",
			config:      types.Configuration{"key1": "value1"},
			paths:       []string{"."},
			expected:    nil,
			expectError: true,
			errorMsg:    "invalid truncate path",
		},
		// Valid cases
		{
			name:        "empty config with valid paths",
			config:      types.Configuration{},
			paths:       []string{"nonexistent"},
			expected:    map[string]any{},
			expectError: false,
		},
		{
			name:   "truncate single top-level key",
			config: types.Configuration{"key1": "value1", "key2": "value2"},
			paths:  []string{"key1"},
			expected: map[string]any{
				"key2": "value2",
			},
			expectError: false,
		},
		{
			name: "truncate nested key",
			config: types.Configuration{
				"database": map[string]any{
					"host":     "localhost",
					"port":     5432,
					"password": "secret",
				},
				"api": map[string]any{
					"timeout": 30,
				},
			},
			paths: []string{"database.password"},
			expected: map[string]any{
				"database": map[string]any{
					"host": "localhost",
					"port": 5432,
				},
				"api": map[string]any{
					"timeout": 30,
				},
			},
			expectError: false,
		},
		{
			name: "truncate multiple paths",
			config: types.Configuration{
				"database": map[string]any{
					"host":     "localhost",
					"port":     5432,
					"password": "secret",
					"username": "admin",
				},
				"api": map[string]any{
					"timeout": 30,
					"secret":  "api-secret",
				},
			},
			paths: []string{"database.password", "database.username", "api.secret"},
			expected: map[string]any{
				"database": map[string]any{
					"host": "localhost",
					"port": 5432,
				},
				"api": map[string]any{
					"timeout": 30,
				},
			},
			expectError: false,
		},
		{
			name: "truncate entire nested object",
			config: types.Configuration{
				"database": map[string]any{
					"credentials": map[string]any{
						"username": "admin",
						"password": "secret",
					},
					"host": "localhost",
				},
				"api": "config",
			},
			paths: []string{"database.credentials"},
			expected: map[string]any{
				"database": map[string]any{
					"host": "localhost",
				},
				"api": "config",
			},
			expectError: false,
		},
		{
			name: "truncate nonexistent path",
			config: types.Configuration{
				"key1": "value1",
				"key2": map[string]any{
					"nested": "value",
				},
			},
			paths: []string{"nonexistent", "key2.nonexistent"},
			expected: map[string]any{
				"key1": "value1",
				"key2": map[string]any{
					"nested": "value",
				},
			},
			expectError: false,
		},
		{
			name: "deep nesting truncation",
			config: types.Configuration{
				"level1": map[string]any{
					"level2": map[string]any{
						"level3": map[string]any{
							"keep":   "this",
							"remove": "this",
						},
						"keep": "this too",
					},
				},
			},
			paths: []string{"level1.level2.level3.remove"},
			expected: map[string]any{
				"level1": map[string]any{
					"level2": map[string]any{
						"level3": map[string]any{
							"keep": "this",
						},
						"keep": "this too",
					},
				},
			},
			expectError: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			output, err := types.TruncateConfiguration(tc.config, tc.paths...)
			if tc.expectError {
				require.Error(t, err)
				if tc.errorMsg != "" {
					require.Contains(t, err.Error(), tc.errorMsg)
				}
				require.Nil(t, output)
			} else {
				require.NoError(t, err)
				require.Empty(t, cmp.Diff(tc.expected, output))
			}
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

func TestMergeRawConfigurationFiles(t *testing.T) {
	testCases := []struct {
		name                          string
		schemaLocationRebaseReference string
		configFiles                   []string
		expectError                   bool
		errorMsg                      string
	}{
		{
			name:        "no config files",
			configFiles: []string{},
			expectError: true,
			errorMsg:    "no configuration files provided",
		},
		{
			name: "single config file",
			configFiles: []string{
				"testdata/config.yaml",
			},
			schemaLocationRebaseReference: "testdata/merge",
			expectError:                   false,
		},
		{
			name: "merge two config files",
			configFiles: []string{
				"testdata/config.yaml",
				"testdata/override.yaml",
			},
			schemaLocationRebaseReference: "testdata/merge",
			expectError:                   false,
		},
		{
			name: "merge two config files with schema override",
			configFiles: []string{
				"testdata/config.yaml",
				"testdata/override.yaml",
				"testdata/nested/override-with-schema.yaml",
			},
			schemaLocationRebaseReference: "testdata/merged",
			expectError:                   false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			output, err := types.MergeRawConfigurationFiles(tc.schemaLocationRebaseReference, tc.configFiles)

			if tc.expectError {
				require.Error(t, err)
				if tc.errorMsg != "" {
					require.Contains(t, err.Error(), tc.errorMsg)
				}
				return
			}

			require.NoError(t, err)
			testutil.CompareWithFixture(t, output)
		})
	}
}
