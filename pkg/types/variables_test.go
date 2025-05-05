package types

import "testing"

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
