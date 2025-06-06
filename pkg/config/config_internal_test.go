package config

import (
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

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
