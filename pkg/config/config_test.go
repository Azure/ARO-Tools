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

package config

import (
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/Azure/ARO-Tools/internal/testutil"
)

func TestConfigProvider(t *testing.T) {
	region := "uksouth"
	regionShort := "uks"
	stamp := "1"

	configProvider := NewConfigProvider("../../testdata/config.yaml")

	config, err := configProvider.GetDeployEnvRegionConfiguration("public", "int", region, NewConfigReplacements(region, regionShort, stamp))
	assert.NoError(t, err)
	assert.NotNil(t, config)

	// key is not in the config file
	assert.Nil(t, config["svc_resourcegroup"])

	// key is in the config file, region constant value
	assert.Equal(t, "uksouth", config["test"])

	// key is in the config file, default in INT, constant value
	assert.Equal(t, "aro-hcp-int.azurecr.io/maestro-server:the-stable-one", config["maestro_image"])

	// key is in the config file, default, varaible value
	assert.Equal(t, fmt.Sprintf("hcp-underlay-%s", regionShort), config["regionRG"])
}

func TestInterfaceToConfiguration(t *testing.T) {
	testCases := []struct {
		name                   string
		i                      interface{}
		ok                     bool
		expecetedConfiguration Configuration
	}{
		{
			name:                   "empty interface",
			ok:                     false,
			expecetedConfiguration: Configuration{},
		},
		{
			name:                   "empty map",
			i:                      map[string]interface{}{},
			ok:                     true,
			expecetedConfiguration: Configuration{},
		},
		{
			name: "map",
			i: map[string]interface{}{
				"key1": "value1",
				"key2": "value2",
			},
			ok: true,
			expecetedConfiguration: Configuration{
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
			expecetedConfiguration: Configuration{
				"key1": Configuration{
					"key2": "value2",
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			vars, ok := InterfaceToConfiguration(tc.i)
			assert.Equal(t, tc.ok, ok)
			assert.Equal(t, tc.expecetedConfiguration, vars)
		})
	}
}

func TestMergeConfiguration(t *testing.T) {
	testCases := []struct {
		name     string
		base     Configuration
		override Configuration
		expected Configuration
	}{
		{
			name:     "nil base",
			expected: nil,
		},
		{
			name:     "empty base and override",
			base:     Configuration{},
			expected: Configuration{},
		},
		{
			name:     "merge into empty base",
			base:     Configuration{},
			override: Configuration{"key1": "value1"},
			expected: Configuration{"key1": "value1"},
		},
		{
			name:     "merge into base",
			base:     Configuration{"key1": "value1"},
			override: Configuration{"key2": "value2"},
			expected: Configuration{"key1": "value1", "key2": "value2"},
		},
		{
			name:     "override base, change schema",
			base:     Configuration{"key1": Configuration{"key2": "value2"}},
			override: Configuration{"key1": "value1"},
			expected: Configuration{"key1": "value1"},
		},
		{
			name:     "merge into sub map",
			base:     Configuration{"key1": Configuration{"key2": "value2"}},
			override: Configuration{"key1": Configuration{"key3": "value3"}},
			expected: Configuration{"key1": Configuration{"key2": "value2", "key3": "value3"}},
		},
		{
			name:     "override sub map value",
			base:     Configuration{"key1": Configuration{"key2": "value2"}},
			override: Configuration{"key1": Configuration{"key2": "value3"}},
			expected: Configuration{"key1": Configuration{"key2": "value3"}},
		},
		{
			name:     "override nested sub map",
			base:     Configuration{"key1": Configuration{"key2": Configuration{"key3": "value3"}}},
			override: Configuration{"key1": Configuration{"key2": Configuration{"key3": "value4"}}},
			expected: Configuration{"key1": Configuration{"key2": Configuration{"key3": "value4"}}},
		},
		{
			name:     "override nested sub map multiple levels",
			base:     Configuration{"key1": Configuration{"key2": Configuration{"key3": "value3"}}},
			override: Configuration{"key1": Configuration{"key2": Configuration{"key4": "value4"}}, "key5": "value5"},
			expected: Configuration{"key1": Configuration{"key2": Configuration{"key3": "value3", "key4": "value4"}}, "key5": "value5"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := MergeConfiguration(tc.base, tc.override)
			assert.Equal(t, tc.expected, result)
		})
	}

}

func TestLoadSchemaURL(t *testing.T) {
	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := fmt.Fprintln(w, "{\"type\": \"object\"}"); err != nil {
			log.Printf("failed to write response: %v", err)
		}
	}))
	defer testServer.Close()

	configProvider := configProviderImpl{}
	configProvider.schema = testServer.URL

	schema, err := configProvider.loadSchema()
	assert.Nil(t, err)
	assert.NotNil(t, schema)
	assert.Equal(t, map[string]any{"type": "object"}, schema)
}

func TestLoadSchema(t *testing.T) {
	testDirs := t.TempDir()

	err := os.WriteFile(testDirs+"/schema.json", []byte(`{"type": "object"}`), 0644)
	assert.Nil(t, err)

	configProvider := configProviderImpl{}
	configProvider.schema = testDirs + "/schema.json"

	schema, err := configProvider.loadSchema()
	assert.Nil(t, err)
	assert.NotNil(t, schema)
	assert.Equal(t, map[string]any{"type": "object"}, schema)
}

func TestLoadSchemaError(t *testing.T) {
	testDirs := t.TempDir()

	err := os.WriteFile(testDirs+"/schma.json", []byte(`{"type": "object"}`), 0644)
	assert.Nil(t, err)

	configProvider := configProviderImpl{}
	configProvider.schema = testDirs + "/schema.json"
	_, err = configProvider.loadSchema()
	assert.NotNil(t, err)
}

func TestValidateSchema(t *testing.T) {
	testSchema := `{
	"type": "object",
	"properties": {
		"key1": {
			"type": "string"
		}
	},
	"additionalProperties": false
}`

	testDirs := t.TempDir()

	err := os.WriteFile(testDirs+"/schema.json", []byte(testSchema), 0644)
	assert.Nil(t, err)

	configProvider := configProviderImpl{}
	configProvider.schema = "schema.json"
	configProvider.config = testDirs + "/config.yaml"

	err = configProvider.validateSchema(map[string]any{"foo": "bar"})
	assert.NotNil(t, err)
	assert.ErrorContains(t, err, "additional properties 'foo' not allowed")

	err = configProvider.validateSchema(map[string]any{"key1": "bar"})
	assert.Nil(t, err)
}

func TestConvertToInterface(t *testing.T) {
	vars := Configuration{
		"key1": "value1",
		"key2": Configuration{
			"key3": "value3",
		},
	}

	expected := map[string]any{
		"key1": "value1",
		"key2": map[string]any{
			"key3": "value3",
		},
	}

	result := convertToInterface(vars)
	assert.Equal(t, expected, result)
	assert.IsType(t, expected, map[string]any{})
	assert.IsType(t, expected["key2"], map[string]any{})
}

func TestPreprocessContent(t *testing.T) {
	fileContent, err := os.ReadFile("../../testdata/test.bicepparam")
	assert.Nil(t, err)

	processed, err := PreprocessContent(
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
			_, err := PreprocessContent(
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
