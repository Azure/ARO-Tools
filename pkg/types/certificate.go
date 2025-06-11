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

import "fmt"

type SetCertificateIssuerStep struct {
	StepMeta     `json:",inline"`
	VaultBaseUrl Variable `json:"vaultBaseUrl,omitempty"`
	Issuer       Variable `json:"issuer,omitempty"`
}

func NewSetCertificateIssuerStep(name string) *SetCertificateIssuerStep {
	return &SetCertificateIssuerStep{
		StepMeta: StepMeta{
			Name:   name,
			Action: "SetCertificateIssuer",
		},
	}
}

// WithDependsOn fluent method that sets DependsOn
func (s *SetCertificateIssuerStep) WithDependsOn(dependsOn ...string) *SetCertificateIssuerStep {
	s.DependsOn = dependsOn
	return s
}

// WithVaultBaseUrl fluent method that sets vaultBaseUrl
func (s *SetCertificateIssuerStep) WithVaultBaseUrl(vaultBaseUrl Variable) *SetCertificateIssuerStep {
	s.VaultBaseUrl = vaultBaseUrl
	return s
}

// WithIssuer fluent method that sets issuer
func (s *SetCertificateIssuerStep) WithIssuer(issuer Variable) *SetCertificateIssuerStep {
	s.Issuer = issuer
	return s
}

func (s *SetCertificateIssuerStep) Description() string {
	return fmt.Sprintf("Step %s\n Kind: %s\n", s.Name, s.Action)
}

type CreateCertificateStep struct {
	StepMeta        `json:",inline"`
	VaultBaseUrl    Variable `json:"vaultBaseUrl,omitempty"`
	CertificateName Variable `json:"certificateName,omitempty"`
	ContentType     Variable `json:"contentType,omitempty"`
	SAN             Variable `json:"san,omitempty"`
	Issuer          Variable `json:"issuer,omitempty"`
}

func NewCreateCertificateStep(name string) *CreateCertificateStep {
	return &CreateCertificateStep{
		StepMeta: StepMeta{
			Name:   name,
			Action: "CreateCertificate",
		},
	}
}

// WithDependsOn fluent method that sets DependsOn
func (s *CreateCertificateStep) WithDependsOn(dependsOn ...string) *CreateCertificateStep {
	s.DependsOn = dependsOn
	return s
}

// WithVaultBaseUrl fluent method that sets vaultBaseUrl
func (s *CreateCertificateStep) WithVaultBaseUrl(vaultBaseUrl Variable) *CreateCertificateStep {
	s.VaultBaseUrl = vaultBaseUrl
	return s
}

// WithCertificateName fluent method that sets certificateName
func (s *CreateCertificateStep) WithCertificateName(certificateName Variable) *CreateCertificateStep {
	s.CertificateName = certificateName
	return s
}

// WithContentType fluent method that sets contentType
func (s *CreateCertificateStep) WithContentType(contentType Variable) *CreateCertificateStep {
	s.ContentType = contentType
	return s
}

// WithSAN fluent method that sets san
func (s *CreateCertificateStep) WithSAN(san Variable) *CreateCertificateStep {
	s.SAN = san
	return s
}

// WithIssuer fluent method that sets issuer
func (s *CreateCertificateStep) WithIssuer(issuer Variable) *CreateCertificateStep {
	s.Issuer = issuer
	return s
}

func (s *CreateCertificateStep) Description() string {
	return fmt.Sprintf("Step %s\n Kind: %s\n", s.Name, s.Action)
}
