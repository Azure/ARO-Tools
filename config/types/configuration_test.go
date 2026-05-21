package types

import (
	"path/filepath"
	"reflect"
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

func TestGetByPathSlices(t *testing.T) {
	cfg := Configuration{
		"scalar": "hello",
		"nested": map[string]any{
			"inner":  "world",
			"number": 42,
		},
		"items": []any{
			"first",
			"second",
			"third",
		},
		"objects": []any{
			map[string]any{
				"name":  "alpha",
				"value": 1,
			},
			map[string]any{
				"name":  "beta",
				"value": 2,
			},
		},
		"matrix": []any{
			[]any{"a", "b"},
			[]any{"c", "d"},
		},
		"acr": map[string]any{
			"extraAllowedRegistries": []any{
				"registry.connect.redhat.com",
				"registry.redhat.io",
				"quay.io/redhat-user-workloads",
			},
		},
	}

	tests := []struct {
		name    string
		path    string
		want    any
		wantErr bool
		// errIs lets us assert a particular sentinel error type.
		// Leave nil to skip the check.
		errIs func(error) bool
	}{
		{
			name: "top-level scalar",
			path: "scalar",
			want: "hello",
		},
		{
			name: "nested map key",
			path: "nested.inner",
			want: "world",
		},
		{
			name: "slice index into top-level slice",
			path: "items.0",
			want: "first",
		},
		{
			name: "slice index, last element",
			path: "items.2",
			want: "third",
		},
		{
			name: "slice index into slice of maps then map key",
			path: "objects.1.name",
			want: "beta",
		},
		{
			name: "nested slice of slices",
			path: "matrix.1.0",
			want: "c",
		},
		{
			name: "realistic: map -> slice index (image-registry-policy shape)",
			path: "acr.extraAllowedRegistries.0",
			want: "registry.connect.redhat.com",
		},
		{
			name:    "missing top-level key",
			path:    "doesNotExist",
			wantErr: true,
			errIs: func(err error) bool {
				_, ok := err.(*MissingKeyError)
				return ok
			},
		},
		{
			name:    "slice index out of range",
			path:    "items.5",
			wantErr: true,
			errIs: func(err error) bool {
				_, ok := err.(*IndexOutOfRangeError)
				return ok
			},
		},
		{
			name:    "negative slice index",
			path:    "items.-1",
			wantErr: true,
			errIs: func(err error) bool {
				_, ok := err.(*IndexOutOfRangeError)
				return ok
			},
		},
		{
			name:    "non-numeric key applied to slice",
			path:    "items.first",
			wantErr: true,
		},
		{
			name:    "navigating into a scalar",
			path:    "scalar.x",
			wantErr: true,
		},
		{
			name: "trailing slice returned whole",
			path: "items",
			want: []any{"first", "second", "third"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := cfg.GetByPath(tt.path)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error for path %q, got nil (value=%v)", tt.path, got)
				}
				if tt.errIs != nil && !tt.errIs(err) {
					t.Fatalf("error type mismatch for path %q: got %T (%v)", tt.path, err, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error for path %q: %v", tt.path, err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("path %q: got %#v, want %#v", tt.path, got, tt.want)
			}
		})
	}
}
