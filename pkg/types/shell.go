package types

import "fmt"

const (
	ev2ShellIdentity = "aroDevopsMsiId"
)

type ShellStep struct {
	// Required fields
	Command string `yaml:"command"`

	// Optional fields
	Variables []Variable `yaml:"variables"`

	ShellIdentity Variable
	AKSCluster    string // AKS Cluster name
}

// Validate validates the Shell step.
func (s *ShellStep) Validate(data map[string]any) error {
	if len(s.Command) == 0 {
		return fmt.Errorf("command is required")
	}

	s.ShellIdentity.ConfigRef = ev2ShellIdentity

	if err := s.ShellIdentity.Validate(data); err != nil {
		return err
	}

	s.Variables = append(s.Variables,
		// Required varaible for azureLogin (.scripts/common.sh)
		Variable{Name: "AzureCloud", ConfigRef: "cloud"},
		// Used by ARO-HCP tooling (goberlec)
		Variable{Name: "EV2", Value: "1"},
	)

	// AKS Cluster release
	if s.AKSCluster != "" {
		s.Variables = append(s.Variables,
			// Required varaible for kubeAuthentication (.scripts/common.sh)
			Variable{Name: "AKSSubscription", Value: "$azureSubscriptionId()"},
			Variable{Name: "AKSResourceGroup", Value: "$azureResourceGroup()"},
			Variable{Name: "AKSCluster", Value: s.AKSCluster},
		)
	}

	for i := range s.Variables {
		if err := s.Variables[i].Validate(data); err != nil {
			return err
		}

		// Plaintext value only
		s.Variables[i].Value = fmt.Sprintf("%v", s.Variables[i].Value)
	}

	return nil
}
