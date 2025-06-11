// Copyright 2025 Microsoft Corporation
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package types

import (
	"testing"

	"github.com/google/go-cmp/cmp"

	"sigs.k8s.io/yaml"
)

func TestNewSetCertificateIssuerStep(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected SetCertificateIssuerStep
		err      bool
	}{
		{
			name: "TestNewSetCertificateIssuerStep_ValidInput",
			input: `
name: test
action: SetCertificateIssuer`,
			expected: *NewSetCertificateIssuerStep("test"),
		},
		{
			name: "TestNewSetCertificateIssuerStep_Complete",
			input: `
name: test
action: SetCertificateIssuer
dependsOn: ["foo-bar"]
vaultBaseURL:
  name: vaultBaseURL
  value: vaultBaseURL
issuer:
  name: issuer
  value: issuer
`,
			expected: *NewSetCertificateIssuerStep("test").
				WithDependsOn("foo-bar").
				WithVaultBaseUrl(Variable{Name: "vaultBaseURL", Value: "vaultBaseURL"}).
				WithIssuer(Variable{Name: "issuer", Value: "issuer"}),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var output SetCertificateIssuerStep

			err := yaml.Unmarshal([]byte(tt.input), &output)
			if (err != nil) != tt.err {
				t.Fatalf("UnmarshalYAML() error = %v, expectedError %v", err, tt.err)
			}

			if diff := cmp.Diff(tt.expected, output, nil); diff != "" {
				t.Fatalf("UnmarshalYAML() mismatch (-expected +got):\n%s", diff)
			}
		})
	}
}

func TestNewCreateCertificateStep(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected CreateCertificateStep
		err      bool
	}{
		{
			name: "TestCreateCertificateStep_ValidInput",
			input: `
name: test
action: CreateCertificate`,
			expected: *NewCreateCertificateStep("test"),
		},
		{
			name: "TestCreateCertificateStep_Complete",
			input: `
name: test
action: CreateCertificate
dependsOn: ["foo-bar"]
vaultBaseURL:
  name: vaultBaseURL
  value: vaultBaseURL
issuer:
  name: issuer
  value: issuer
certificateName:
  name: certificateName
  value: certificateName
contentType:
  name: contentType
  value: contentType
san:
  name: san
  value: san
`,
			expected: *NewCreateCertificateStep("test").
				WithDependsOn("foo-bar").
				WithVaultBaseUrl(Variable{Name: "vaultBaseURL", Value: "vaultBaseURL"}).
				WithIssuer(Variable{Name: "issuer", Value: "issuer"}).
				WithCertificateName(Variable{Name: "certificateName", Value: "certificateName"}).
				WithContentType(Variable{Name: "contentType", Value: "contentType"}).
				WithSAN(Variable{Name: "san", Value: "san"}),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var output CreateCertificateStep

			err := yaml.Unmarshal([]byte(tt.input), &output)
			if (err != nil) != tt.err {
				t.Fatalf("UnmarshalYAML() error = %v, expectedError %v", err, tt.err)
			}

			if diff := cmp.Diff(tt.expected, output, nil); diff != "" {
				t.Fatalf("UnmarshalYAML() mismatch (-expected +got):\n%s", diff)
			}
		})
	}
}
