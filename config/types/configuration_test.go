package types

import (
	"path/filepath"
	"testing"
)

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
