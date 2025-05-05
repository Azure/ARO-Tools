package types

import (
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestShellValidate(t *testing.T) {
	tests := []struct {
		name      string
		shell     *ShellStep
		variables []Variable
		err       bool
	}{
		{
			name:      "Invalid command",
			shell:     &ShellStep{},
			variables: nil,
			err:       true,
		},
		{
			name: "Invalid variable",
			shell: &ShellStep{
				Command:   "make test",
				Variables: []Variable{{ConfigRef: "test"}},
			},
			variables: []Variable{
				{ConfigRef: "test"},
				{Name: "AzureCloud", ConfigRef: "cloud"},
				{Name: "EV2", Value: "1"},
			},
			err: true,
		},
		{
			name: "Valid data",
			shell: &ShellStep{
				Command:    "make test",
				AKSCluster: "aks",
				Variables:  []Variable{{Name: "bool", ConfigRef: "bool"}},
			},
			variables: []Variable{
				{Name: "bool", ConfigRef: "bool", Value: "true"},
				{Name: "AzureCloud", ConfigRef: "cloud", Value: "TestCloud"},
				{Name: "EV2", Value: "1"},
				{Name: "AKSSubscription", Value: "$azureSubscriptionId()"},
				{Name: "AKSResourceGroup", Value: "$azureResourceGroup()"},
				{Name: "AKSCluster", Value: "aks"},
			},
			err: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.shell.Validate(map[string]any{
				"cloud":          "TestCloud",
				"aroDevopsMsiId": "testid",
				"bool":           true,
			})

			if cmp.Diff(tt.variables, tt.shell.Variables) != "" {
				t.Fatalf("Validate() values are not expected: %s", cmp.Diff(tt.variables, tt.shell.Variables))
			}

			if (err != nil) != tt.err {
				t.Fatalf("Validate() error = %v, expectedError %v", err, err)
			}
		})
	}
}
