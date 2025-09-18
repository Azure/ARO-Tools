package types

import (
	"fmt"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestRequiredInputs(t *testing.T) {
	for _, testCase := range []struct {
		name     string
		input    Step
		expected []StepDependency
	}{
		{
			name: "shell full",
			input: &ShellStep{
				Variables: []Variable{
					{Value: Value{Input: &Input{StepDependency: StepDependency{ResourceGroup: "rg", Step: "step"}}}},
					{Value: Value{Input: &Input{StepDependency: StepDependency{ResourceGroup: "rg2", Step: "step2"}}}},
				},
				ShellIdentity: Value{Input: &Input{StepDependency: StepDependency{ResourceGroup: "rg", Step: "step"}}},
			},
			expected: []StepDependency{
				{ResourceGroup: "rg", Step: "step"},
				{ResourceGroup: "rg2", Step: "step2"},
			},
		},
		{
			name:  "shell empty",
			input: &ShellStep{},
		},
		{
			name: "arm full",
			input: &ARMStep{
				Variables: []Variable{
					{Value: Value{Input: &Input{StepDependency: StepDependency{ResourceGroup: "rg", Step: "step"}}}},
					{Value: Value{Input: &Input{StepDependency: StepDependency{ResourceGroup: "rg2", Step: "step2"}}}},
				},
			},
			expected: []StepDependency{
				{ResourceGroup: "rg", Step: "step"},
				{ResourceGroup: "rg2", Step: "step2"},
			},
		},
		{
			name:  "arm empty",
			input: &ARMStep{},
		},
		{
			name: "delegate full",
			input: &DelegateChildZoneStep{
				ParentZone: Value{Input: &Input{StepDependency: StepDependency{ResourceGroup: "rg", Step: "step"}}},
				ChildZone:  Value{Input: &Input{StepDependency: StepDependency{ResourceGroup: "rg2", Step: "step2"}}},
			},
			expected: []StepDependency{
				{ResourceGroup: "rg", Step: "step"},
				{ResourceGroup: "rg2", Step: "step2"},
			},
		},
		{
			name:  "delegate empty",
			input: &DelegateChildZoneStep{},
		},
		{
			name: "issuer full",
			input: &SetCertificateIssuerStep{
				VaultBaseUrl:   Value{Input: &Input{StepDependency: StepDependency{ResourceGroup: "rg", Step: "step"}}},
				Issuer:         Value{Input: &Input{StepDependency: StepDependency{ResourceGroup: "rg", Step: "step2"}}},
				SecretKeyVault: Value{Input: &Input{StepDependency: StepDependency{ResourceGroup: "rg", Step: "step3"}}},
				SecretName:     Value{Input: &Input{StepDependency: StepDependency{ResourceGroup: "rg", Step: "step3"}}},
				ApplicationId:  Value{Input: &Input{StepDependency: StepDependency{ResourceGroup: "rg", Step: "step4"}}},
			},
			expected: []StepDependency{
				{ResourceGroup: "rg", Step: "step"},
				{ResourceGroup: "rg", Step: "step2"},
				{ResourceGroup: "rg", Step: "step3"},
				{ResourceGroup: "rg", Step: "step4"},
			},
		},
		{
			name:  "issuer empty",
			input: &SetCertificateIssuerStep{},
		},
		{
			name: "cert full",
			input: &CreateCertificateStep{
				VaultBaseUrl:    Value{Input: &Input{StepDependency: StepDependency{ResourceGroup: "rg", Step: "step"}}},
				CertificateName: Value{Input: &Input{StepDependency: StepDependency{ResourceGroup: "rg", Step: "step2"}}},
				ContentType:     Value{Input: &Input{StepDependency: StepDependency{ResourceGroup: "rg", Step: "step3"}}},
				SAN:             Value{Input: &Input{StepDependency: StepDependency{ResourceGroup: "rg", Step: "step4"}}},
				Issuer:          Value{Input: &Input{StepDependency: StepDependency{ResourceGroup: "rg", Step: "step4"}}},
				SecretKeyVault:  Value{Input: &Input{StepDependency: StepDependency{ResourceGroup: "rg", Step: "step5"}}},
				SecretName:      Value{Input: &Input{StepDependency: StepDependency{ResourceGroup: "rg", Step: "step6"}}},
				ApplicationId:   Value{Input: &Input{StepDependency: StepDependency{ResourceGroup: "rg", Step: "step7"}}},
				CommonName:      Value{Input: &Input{StepDependency: StepDependency{ResourceGroup: "rg", Step: "step8"}}},
			},
			expected: []StepDependency{
				{ResourceGroup: "rg", Step: "step"},
				{ResourceGroup: "rg", Step: "step2"},
				{ResourceGroup: "rg", Step: "step3"},
				{ResourceGroup: "rg", Step: "step4"},
				{ResourceGroup: "rg", Step: "step5"},
				{ResourceGroup: "rg", Step: "step6"},
				{ResourceGroup: "rg", Step: "step7"},
				{ResourceGroup: "rg", Step: "step8"},
			},
		},
		{
			name:  "cert empty",
			input: &CreateCertificateStep{},
		},
		{
			name: "rp full",
			input: &ResourceProviderRegistrationStep{
				ResourceProviderNamespaces: Value{Input: &Input{StepDependency: StepDependency{ResourceGroup: "rg", Step: "step"}}},
			},
			expected: []StepDependency{
				{ResourceGroup: "rg", Step: "step"},
			},
		},
		{
			name:  "rp empty",
			input: &ResourceProviderRegistrationStep{},
		},
		{
			name: "image mirror full",
			input: &ImageMirrorStep{
				TargetACR:          Value{Input: &Input{StepDependency: StepDependency{ResourceGroup: "rg", Step: "step"}}},
				SourceRegistry:     Value{Input: &Input{StepDependency: StepDependency{ResourceGroup: "rg", Step: "step2"}}},
				Repository:         Value{Input: &Input{StepDependency: StepDependency{ResourceGroup: "rg", Step: "step3"}}},
				Digest:             Value{Input: &Input{StepDependency: StepDependency{ResourceGroup: "rg", Step: "step4"}}},
				PullSecretKeyVault: Value{Input: &Input{StepDependency: StepDependency{ResourceGroup: "rg", Step: "step1"}}},
				PullSecretName:     Value{Input: &Input{StepDependency: StepDependency{ResourceGroup: "rg", Step: "step1"}}},
				ShellIdentity:      Value{Input: &Input{StepDependency: StepDependency{ResourceGroup: "rg", Step: "step2"}}},
			},
			expected: []StepDependency{
				{ResourceGroup: "rg", Step: "step"},
				{ResourceGroup: "rg", Step: "step1"},
				{ResourceGroup: "rg", Step: "step2"},
				{ResourceGroup: "rg", Step: "step3"},
				{ResourceGroup: "rg", Step: "step4"},
			},
		},
		{
			name:  "image mirror empty",
			input: &ImageMirrorStep{},
		},
		{
			name: "logs full",
			input: &LogsStep{
				RolloutKind:     "FluentBit",
				TypeName:        Value{Input: &Input{StepDependency: StepDependency{ResourceGroup: "rg", Step: "step"}}},
				SecretKeyVault:  Value{Input: &Input{StepDependency: StepDependency{ResourceGroup: "rg", Step: "step2"}}},
				SecretName:      Value{Input: &Input{StepDependency: StepDependency{ResourceGroup: "rg", Step: "step2"}}},
				Environment:     Value{Input: &Input{StepDependency: StepDependency{ResourceGroup: "rg", Step: "step3"}}},
				AccountName:     Value{Input: &Input{StepDependency: StepDependency{ResourceGroup: "rg", Step: "step3"}}},
				MetricsAccount:  Value{Input: &Input{StepDependency: StepDependency{ResourceGroup: "rg", Step: "step4"}}},
				AdminAlias:      Value{Input: &Input{StepDependency: StepDependency{ResourceGroup: "rg", Step: "step5"}}},
				AdminGroup:      Value{Input: &Input{StepDependency: StepDependency{ResourceGroup: "rg", Step: "step6"}}},
				SubscriptionId:  Value{Input: &Input{StepDependency: StepDependency{ResourceGroup: "rg", Step: "step7"}}},
				Namespace:       Value{Input: &Input{StepDependency: StepDependency{ResourceGroup: "rg", Step: "step8"}}},
				CertSAN:         Value{Input: &Input{StepDependency: StepDependency{ResourceGroup: "rg", Step: "step8"}}},
				CertDescription: Value{Input: &Input{StepDependency: StepDependency{ResourceGroup: "rg", Step: "step9"}}},
				ConfigVersion:   Value{Input: &Input{StepDependency: StepDependency{ResourceGroup: "rg", Step: "step9"}}},
			},
			expected: []StepDependency{
				{ResourceGroup: "rg", Step: "step"},
				{ResourceGroup: "rg", Step: "step2"},
				{ResourceGroup: "rg", Step: "step3"},
				{ResourceGroup: "rg", Step: "step4"},
				{ResourceGroup: "rg", Step: "step5"},
				{ResourceGroup: "rg", Step: "step6"},
				{ResourceGroup: "rg", Step: "step7"},
				{ResourceGroup: "rg", Step: "step8"},
				{ResourceGroup: "rg", Step: "step9"},
			},
		},
		{
			name:  "logs empty",
			input: &LogsStep{},
		},
		{
			name: "feature full",
			input: &FeatureRegistrationStep{
				SecretKeyVault: Value{Input: &Input{StepDependency: StepDependency{ResourceGroup: "rg", Step: "step"}}},
				SecretName:     Value{Input: &Input{StepDependency: StepDependency{ResourceGroup: "rg", Step: "step2"}}},
			},
			expected: []StepDependency{
				{ResourceGroup: "rg", Step: "step"},
				{ResourceGroup: "rg", Step: "step2"},
			},
		},
		{
			name:  "feature empty",
			input: &FeatureRegistrationStep{},
		},
		{
			name: "provider full",
			input: &ProviderFeatureRegistrationStep{
				IdentityFrom: Input{StepDependency: StepDependency{ResourceGroup: "rg", Step: "step"}},
			},
			expected: []StepDependency{
				{ResourceGroup: "rg", Step: "step"},
			},
		},
		{
			name: "ev2 full",
			input: &Ev2RegistrationStep{
				IdentityFrom: Input{StepDependency: StepDependency{ResourceGroup: "rg", Step: "step"}},
			},
			expected: []StepDependency{
				{ResourceGroup: "rg", Step: "step"},
			},
		},
		{
			name: "secret full",
			input: &SecretSyncStep{
				IdentityFrom: Input{StepDependency: StepDependency{ResourceGroup: "rg", Step: "step"}},
			},
			expected: []StepDependency{
				{ResourceGroup: "rg", Step: "step"},
			},
		},
		{
			name: "kusto full",
			input: &KustoStep{
				SecretKeyVault:   Value{Input: &Input{StepDependency: StepDependency{ResourceGroup: "rg", Step: "step"}}},
				SecretName:       Value{Input: &Input{StepDependency: StepDependency{ResourceGroup: "rg", Step: "step2"}}},
				ApplicationId:    Value{Input: &Input{StepDependency: StepDependency{ResourceGroup: "rg", Step: "step3"}}},
				ConnectionString: Value{Input: &Input{StepDependency: StepDependency{ResourceGroup: "rg", Step: "step4"}}},
				Command:          Value{Input: &Input{StepDependency: StepDependency{ResourceGroup: "rg", Step: "step6"}}},
			},
			expected: []StepDependency{
				{ResourceGroup: "rg", Step: "step"},
				{ResourceGroup: "rg", Step: "step2"},
				{ResourceGroup: "rg", Step: "step3"},
				{ResourceGroup: "rg", Step: "step4"},
				{ResourceGroup: "rg", Step: "step6"},
			},
		},
		{
			name:  "kusto empty",
			input: &KustoStep{},
		},
		{
			name: "pav2 full",
			input: &Pav2Step{
				SecretKeyVault:             Value{Input: &Input{StepDependency: StepDependency{ResourceGroup: "rg", Step: "step"}}},
				SecretName:                 Value{Input: &Input{StepDependency: StepDependency{ResourceGroup: "rg", Step: "step2"}}},
				StorageAccount:             Value{Input: &Input{StepDependency: StepDependency{ResourceGroup: "rg", Step: "step3"}}},
				SMEEndpointSuffixParameter: Value{Input: &Input{StepDependency: StepDependency{ResourceGroup: "rg", Step: "step"}}},
				SMEAppidParameter:          Value{Input: &Input{StepDependency: StepDependency{ResourceGroup: "rg", Step: "step"}}},
			},
			expected: []StepDependency{
				{ResourceGroup: "rg", Step: "step"},
				{ResourceGroup: "rg", Step: "step2"},
				{ResourceGroup: "rg", Step: "step3"},
			},
		},
		{
			name:  "pav2 empty",
			input: &Pav2Step{},
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			if diff := cmp.Diff(testCase.expected, testCase.input.RequiredInputs()); diff != "" {
				t.Errorf("required inputs mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestIsWellFormedOverInputs(t *testing.T) {
	for _, testCase := range []struct {
		in       WellFormedChecker
		expected bool
	}{
		{in: &ShellStep{}, expected: false},
		{in: &HelmStep{}, expected: true},
		{in: &ARMStep{}, expected: true},
		{in: &ARMStackStep{}, expected: true},
		{in: &DelegateChildZoneStep{}, expected: true},
		{in: &SetCertificateIssuerStep{}, expected: true},
		{in: &CreateCertificateStep{}, expected: true},
		{in: &ResourceProviderRegistrationStep{}, expected: true},
		{in: &ImageMirrorStep{}, expected: true},
		{in: &ImageMirrorStep{CopyFrom: "oci-layout"}, expected: false},
		{in: &LogsStep{}, expected: true},
		{in: &FeatureRegistrationStep{}, expected: true},
		{in: &ProviderFeatureRegistrationStep{}, expected: true},
		{in: &Ev2RegistrationStep{}, expected: true},
		{in: &SecretSyncStep{}, expected: true},
		{in: &KustoStep{}, expected: true},
		{in: &Pav2Step{}, expected: true},
		{in: &GenericStep{}, expected: false},
		{in: &GenericValidationStep{}, expected: false},
		{in: &ShellValidationStep{}, expected: false},
	} {
		t.Run(fmt.Sprintf("%T", testCase.in)[1:], func(t *testing.T) {
			if got, expected := testCase.in.IsWellFormedOverInputs(), testCase.expected; got != expected {
				t.Errorf("IsWellFormedOverInputs() = %v, want %v", got, testCase.expected)
			}
		})
	}
}
