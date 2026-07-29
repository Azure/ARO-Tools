package types

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"testing"
	"text/template"

	"sigs.k8s.io/yaml"
)

func TestConfiguration_UnmarshalJSON_NestedAndArrays(t *testing.T) {
	input := `{
		"nested": {"count": 5000000},
		"items": [3000000, 1.5, "text"]
	}`

	var cfg Configuration
	if err := json.Unmarshal([]byte(input), &cfg); err != nil {
		t.Fatalf("UnmarshalJSON failed: %v", err)
	}

	nested, ok := cfg["nested"].(map[string]any)
	if !ok {
		t.Fatalf("nested is %T, want map[string]any", cfg["nested"])
	}
	if v := nested["count"]; v != int64(5000000) {
		t.Errorf("nested.count = %v (%T), want int64(5000000)", v, v)
	}

	items, ok := cfg["items"].([]any)
	if !ok {
		t.Fatalf("items is %T, want []any", cfg["items"])
	}
	if items[0] != int64(3000000) {
		t.Errorf("items[0] = %v (%T), want int64(3000000)", items[0], items[0])
	}
	if items[1] != float64(1.5) {
		t.Errorf("items[1] = %v (%T), want float64(1.5)", items[1], items[1])
	}
	if items[2] != "text" {
		t.Errorf("items[2] = %v (%T), want \"text\"", items[2], items[2])
	}
}

func TestConfiguration_UnmarshalJSON_Null(t *testing.T) {
	var cfg Configuration
	if err := json.Unmarshal([]byte("null"), &cfg); err != nil {
		t.Fatalf("UnmarshalJSON failed: %v", err)
	}
	if cfg != nil {
		t.Fatalf("cfg = %#v, want nil", cfg)
	}
}

func TestConfiguration_TemplateRendering_NoScientificNotation(t *testing.T) {
	input := `largeInt: 2000000`
	var cfg Configuration
	if err := yaml.Unmarshal([]byte(input), &cfg); err != nil {
		t.Fatalf("yaml.Unmarshal failed: %v", err)
	}

	tmpl := template.Must(template.New("t").Parse("{{ .largeInt }}"))
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, map[string]any(cfg)); err != nil {
		t.Fatalf("template.Execute failed: %v", err)
	}
	if got := buf.String(); got != "2000000" {
		t.Errorf("template rendered %q, want %q", got, "2000000")
	}
}

func TestResolveSchemaPath(t *testing.T) {
	tests := []struct {
		name           string
		schemaPath     string
		originalDir    string
		targetDir      string
		expectedResult string
		expectError    bool
	}{
		{
			name:           "absolute path should be preserved",
			schemaPath:     "/absolute/path/schema.json",
			originalDir:    "/original",
			targetDir:      "/target",
			expectedResult: "/absolute/path/schema.json",
			expectError:    false,
		},
		{
			name:           "relative path in same directory",
			schemaPath:     "schema.json",
			originalDir:    "/base",
			targetDir:      "/output",
			expectedResult: "../base/schema.json",
			expectError:    false,
		},
		{
			name:           "relative path in subdirectory",
			schemaPath:     "schemas/main.json",
			originalDir:    "/base",
			targetDir:      "/output",
			expectedResult: "../base/schemas/main.json",
			expectError:    false,
		},
		{
			name:           "relative path in parent directory",
			schemaPath:     "../shared/schema.json",
			originalDir:    "/base/subdir",
			targetDir:      "/output",
			expectedResult: "../base/shared/schema.json",
			expectError:    false,
		},
		{
			name:           "relative path with multiple parent directories",
			schemaPath:     "../../schemas/schema.json",
			originalDir:    "/base/subdir/nested",
			targetDir:      "/output",
			expectedResult: "../base/schemas/schema.json",
			expectError:    false,
		},
		{
			name:           "target in same directory as original",
			schemaPath:     "schema.json",
			originalDir:    "/base",
			targetDir:      "/base",
			expectedResult: "schema.json",
			expectError:    false,
		},
		{
			name:           "target in subdirectory of original",
			schemaPath:     "schema.json",
			originalDir:    "/base",
			targetDir:      "/base/subdir",
			expectedResult: "../schema.json",
			expectError:    false,
		},
		{
			name:           "complex relative path",
			schemaPath:     "../schemas/v1/config.json",
			originalDir:    "/project/configs/base",
			targetDir:      "/project/output",
			expectedResult: "../configs/schemas/v1/config.json",
			expectError:    false,
		},
		{
			name:           "empty schema path",
			schemaPath:     "",
			originalDir:    "/base",
			targetDir:      "/output",
			expectedResult: "",
			expectError:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := resolveSchemaPath(tt.schemaPath, tt.originalDir, tt.targetDir)

			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			// Normalize paths for comparison
			expected := filepath.Clean(tt.expectedResult)
			actual := filepath.Clean(result)

			if expected != actual {
				t.Errorf("Expected %q, got %q", expected, actual)
			}
		})
	}
}
