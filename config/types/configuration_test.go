package types

import (
	"fmt"
	"path/filepath"
	"reflect"
	"testing"

	"sigs.k8s.io/yaml"
)

func TestConfigurationUnmarshalPreservesIntegerTypes(t *testing.T) {
	type wrapper struct {
		Defaults Configuration `json:"defaults"`
	}

	tests := []struct {
		name       string
		yaml       string
		path       string
		wantVal    any
		wantSprint string
	}{
		{
			name:       "small integer",
			yaml:       "defaults:\n  val: 1024",
			path:       "val",
			wantVal:    int64(1024),
			wantSprint: "1024",
		},
		{
			name:       "large integer no scientific notation",
			yaml:       "defaults:\n  val: 2000000",
			path:       "val",
			wantVal:    int64(2000000),
			wantSprint: "2000000",
		},
		{
			name:       "zero",
			yaml:       "defaults:\n  val: 0",
			path:       "val",
			wantVal:    int64(0),
			wantSprint: "0",
		},
		{
			name:       "negative integer",
			yaml:       "defaults:\n  val: -42",
			path:       "val",
			wantVal:    int64(-42),
			wantSprint: "-42",
		},
		{
			name:       "float with decimal",
			yaml:       "defaults:\n  val: 3.14",
			path:       "val",
			wantVal:    float64(3.14),
			wantSprint: "3.14",
		},
		{
			name:       "large float still uses scientific notation",
			yaml:       "defaults:\n  val: 2000000.5",
			path:       "val",
			wantVal:    float64(2000000.5),
			wantSprint: "2.0000005e+06",
		},
		{
			name:       "string",
			yaml:       "defaults:\n  val: hello",
			path:       "val",
			wantVal:    "hello",
			wantSprint: "hello",
		},
		{
			name:       "boolean",
			yaml:       "defaults:\n  val: true",
			path:       "val",
			wantVal:    true,
			wantSprint: "true",
		},
		{
			name:       "nested integer",
			yaml:       "defaults:\n  outer:\n    inner: 4000000",
			path:       "outer.inner",
			wantVal:    int64(4000000),
			wantSprint: "4000000",
		},
		{
			name:       "deeply nested integer",
			yaml:       "defaults:\n  a:\n    b:\n      c: 9999999",
			path:       "a.b.c",
			wantVal:    int64(9999999),
			wantSprint: "9999999",
		},
		{
			name:       "array with integers",
			yaml:       "defaults:\n  val:\n    - 1\n    - 2000000\n    - 3",
			path:       "val",
			wantVal:    []any{int64(1), int64(2000000), int64(3)},
			wantSprint: "[1 2000000 3]",
		},
		{
			name:       "array with nested objects",
			yaml:       "defaults:\n  val:\n    - num: 2000000",
			path:       "val",
			wantVal:    []any{map[string]any{"num": int64(2000000)}},
			wantSprint: "[map[num:2000000]]",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var w wrapper
			if err := yaml.Unmarshal([]byte(tt.yaml), &w); err != nil {
				t.Fatalf("yaml.Unmarshal: %v", err)
			}

			val, err := w.Defaults.GetByPath(tt.path)
			if err != nil {
				t.Fatalf("GetByPath(%q): %v", tt.path, err)
			}

			if !reflect.DeepEqual(val, tt.wantVal) {
				t.Errorf("value: got %v (%T), want %v (%T)", val, val, tt.wantVal, tt.wantVal)
			}

			rendered := fmt.Sprint(val)
			if rendered != tt.wantSprint {
				t.Errorf("Sprint: got %q, want %q", rendered, tt.wantSprint)
			}
		})
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
