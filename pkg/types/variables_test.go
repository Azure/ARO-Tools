package types

import (
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestValidate(t *testing.T) {
	tests := []struct {
		name     string
		variable Variable
		data     map[string]any
		value    any
		err      bool
	}{
		{
			name:     "Empty variable",
			variable: Variable{},
			data:     nil,
			value:    nil,
			err:      true,
		},
		{
			name:     "Configuration key not found",
			variable: Variable{ConfigRef: "test"},
			data:     map[string]any{"hello": "world"},
			value:    nil,
			err:      true,
		},
		{
			name:     "Configuration key found",
			variable: Variable{ConfigRef: "hello"},
			data:     map[string]any{"hello": "world"},
			value:    "world",
			err:      false,
		},
		{
			name:     "Configuration system variable",
			variable: Variable{ConfigRef: "stamp"},
			data:     map[string]interface{}{"stamp": "test-$stamp()"},
			value:    "test-$stamp()",
			err:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.variable.Validate(tt.data)
			if (err != nil) != tt.err {
				t.Fatalf("Validate() error = %v, expectedError %v", err, err)
			}

			if tt.variable.Value != tt.value {
				t.Fatalf("Validate() value error = %v, expectedError %v", err, err)
			}
		})
	}
}

func TestGetConfigValue(t *testing.T) {
	tests := []struct {
		name   string
		data   map[string]any
		key    string
		split  string
		output any
		err    bool
	}{
		{
			name:   "Empty data",
			data:   nil,
			key:    "",
			output: nil,
			err:    true,
		},
		{
			name:   "Key not found",
			data:   map[string]any{"test": "test"},
			key:    "test1",
			output: nil,
			err:    true,
		},
		{
			name:   "Simple key found",
			data:   map[string]any{"test": "test"},
			key:    "test",
			output: "test",
			err:    false,
		},
		{
			name:   "Complex key found",
			data:   map[string]any{"test1": map[string]any{"test2": "test"}},
			key:    "test1.test2",
			output: "test",
			err:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := getConfigValue(tt.data, tt.key)
			if (err != nil) != tt.err {
				t.Fatalf("getConfigValue() error = %v, expectedError %v", err, err)
			}

			if diff := cmp.Diff(tt.output, result); diff != "" {
				t.Fatalf("getConfigValue() mismatch (-expected +got):\n%s", diff)
			}
		})
	}
}
