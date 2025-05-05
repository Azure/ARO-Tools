package types

import "testing"

func TestARMValidate(t *testing.T) {
	tests := []struct {
		name string
		arm  *ARMStep
		err  bool
	}{
		{
			name: "Invalid Template",
			arm: &ARMStep{
				Template: "test.json",
			},
			err: true,
		},
		{
			name: "Invalid Parameters",
			arm: &ARMStep{
				Parameters: "test.json",
			},
			err: true,
		},
		{
			name: "Invalid DeploymentLevel",
			arm: &ARMStep{
				DeploymentLevel: "test",
			},
			err: true,
		},
		{
			name: "Valid data",
			arm: &ARMStep{
				Template:        "test.bicep",
				Parameters:      "test.bicepparam",
				DeploymentLevel: "Subscription",
			},
			err: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.arm.Validate(map[string]any{})

			if (err != nil) != tt.err {
				t.Fatalf("Validate() error = %v, expectedError %v", err, err)
			}
		})
	}
}
