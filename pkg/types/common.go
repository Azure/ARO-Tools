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
	"fmt"
)

type Step interface {
	StepName() string
	ActionType() string
	Description() string
	Dependencies() []string
}

// StepMeta contains metadata for a steps.
type StepMeta struct {
	Name      string   `json:"name"`
	Action    string   `json:"action"`
	DependsOn []string `json:"dependsOn,omitempty"`
}

func (m *StepMeta) StepName() string {
	return m.Name
}

func (m *StepMeta) ActionType() string {
	return m.Action
}

func (m *StepMeta) Dependencies() []string {
	return m.DependsOn
}

type GenericStep struct {
	StepMeta `json:",inline"`
}

func (s *GenericStep) Description() string {
	return fmt.Sprintf("Step %s\n  Kind: %s", s.Name, s.Action)
}

type DryRun struct {
	Variables []Variable `json:"variables,omitempty"`
	Command   string     `json:"command,omitempty"`
}

type DelegateChildZoneStep struct {
	StepMeta   `json:",inline"`
	ParentZone Value `json:"parentZone,omitempty"`
	ChildZone  Value `json:"childZone,omitempty"`
}

func (s *DelegateChildZoneStep) Description() string {
	return fmt.Sprintf("Step %s\n Kind: %s\n", s.Name, s.Action)
}

type SetCertificateIssuerStep struct {
	StepMeta      `json:",inline"`
	VaultBaseUrl  Value `json:"vaultBaseUrl,omitempty"`
	Issuer        Value `json:"issuer,omitempty"`
	SecretId      Value `json:"secretId,omitempty"`
	ApplicationId Value `json:"applicationId,omitempty"`
}

func (s *SetCertificateIssuerStep) Description() string {
	return fmt.Sprintf("Step %s\n Kind: %s\n", s.Name, s.Action)
}

type CreateCertificateStep struct {
	StepMeta        `json:",inline"`
	VaultBaseUrl    Value `json:"vaultBaseUrl,omitempty"`
	CertificateName Value `json:"certificateName,omitempty"`
	ContentType     Value `json:"contentType,omitempty"`
	SAN             Value `json:"san,omitempty"`
	Issuer          Value `json:"issuer,omitempty"`
	SecretId        Value `json:"secretId,omitempty"`
	ApplicationId   Value `json:"applicationId,omitempty"`
}

func (s *CreateCertificateStep) Description() string {
	return fmt.Sprintf("Step %s\n Kind: %s\n", s.Name, s.Action)
}

type ResourceProviderRegistrationStep struct {
	StepMeta                   `json:",inline"`
	ResourceProviderNamespaces Value `json:"resourceProviderNamespaces,omitempty"`
}

func (s *ResourceProviderRegistrationStep) Description() string {
	return fmt.Sprintf("Step %s\n Kind: %s\n", s.Name, s.Action)
}

type LogsStep struct {
	StepMeta        `json:",inline"`
	TypeName        Value             `json:"typeName"`
	SecretId        Value             `json:"secretId"`
	Environment     Value             `json:"environment"`
	AccountName     Value             `json:"accountName"`
	MetricsAccount  Value             `json:"metricsAccount"`
	AdminAlias      Value             `json:"adminAlias"`
	AdminGroup      Value             `json:"adminGroup"`
	SubscriptionId  Value             `json:"subscriptionId,omitempty"`
	Namespace       Value             `json:"namespace,omitempty"`
	CertSAN         Value             `json:"certsan,omitempty"`
	CertDescription Value             `json:"certdescription,omitempty"`
	ConfigVersion   Value             `json:"configVersion,omitempty"`
	Events          map[string]string `json:"events,omitempty"`
}

func (s *LogsStep) Description() string {
	return fmt.Sprintf("Step %s\n Kind: %s\n", s.Name, s.Action)
}
