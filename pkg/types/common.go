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
	"slices"
	"strings"
)

type Step interface {
	StepName() string
	ActionType() string
	Description() string
	Dependencies() []StepDependency
	RequiredInputs() []StepDependency
}

// StepMeta contains metadata for a steps.
type StepMeta struct {
	Name      string           `json:"name"`
	Action    string           `json:"action"`
	DependsOn []StepDependency `json:"dependsOn,omitempty"`
}

// StepDependency describes a step that must run before the dependent step may begin.
type StepDependency struct {
	// ResourceGroup is the (semantic/display) name of the group to which the step belongs.
	ResourceGroup string `json:"resourceGroup"`
	// Step is the name of the step being depended on.
	Step string `json:"step"`
}

func SortDependencies(a, b StepDependency) int {
	if cmp := strings.Compare(a.ResourceGroup, b.ResourceGroup); cmp != 0 {
		return cmp
	}
	return strings.Compare(a.Step, b.Step)
}

func (m *StepMeta) StepName() string {
	return m.Name
}

func (m *StepMeta) ActionType() string {
	return m.Action
}

func (m *StepMeta) Dependencies() []StepDependency {
	return m.DependsOn
}

type GenericStep struct {
	StepMeta `json:",inline"`
}

func (s *GenericStep) Description() string {
	return fmt.Sprintf("Step %s\n  Kind: %s", s.Name, s.Action)
}

func (s *GenericStep) RequiredInputs() []StepDependency {
	return []StepDependency{}
}

type DryRun struct {
	Variables []Variable `json:"variables,omitempty"`
	Command   string     `json:"command,omitempty"`
}

const StepActionDelegateChildZone = "DelegateChildZone"

type DelegateChildZoneStep struct {
	StepMeta   `json:",inline"`
	ParentZone Value `json:"parentZone,omitempty"`
	ChildZone  Value `json:"childZone,omitempty"`
}

func (s *DelegateChildZoneStep) Description() string {
	return fmt.Sprintf("Step %s\n Kind: %s\n", s.Name, s.Action)
}

func (s *DelegateChildZoneStep) RequiredInputs() []StepDependency {
	var deps []StepDependency
	for _, val := range []Value{s.ParentZone, s.ChildZone} {
		if val.Input != nil {
			deps = append(deps, val.Input.StepDependency)
		}
	}
	slices.SortFunc(deps, SortDependencies)
	deps = slices.Compact(deps)
	return deps
}

const StepActionSetCertificateIssuer = "SetCertificateIssuer"

type SetCertificateIssuerStep struct {
	StepMeta       `json:",inline"`
	VaultBaseUrl   Value `json:"vaultBaseUrl,omitempty"`
	Issuer         Value `json:"issuer,omitempty"`
	SecretKeyVault Value `json:"secretKeyVault,omitempty"`
	SecretName     Value `json:"secretName,omitempty"`
	ApplicationId  Value `json:"applicationId,omitempty"`
}

func (s *SetCertificateIssuerStep) Description() string {
	return fmt.Sprintf("Step %s\n Kind: %s\n", s.Name, s.Action)
}

func (s *SetCertificateIssuerStep) RequiredInputs() []StepDependency {
	var deps []StepDependency
	for _, val := range []Value{s.VaultBaseUrl, s.Issuer, s.SecretKeyVault, s.SecretName, s.ApplicationId} {
		if val.Input != nil {
			deps = append(deps, val.Input.StepDependency)
		}
	}
	slices.SortFunc(deps, SortDependencies)
	deps = slices.Compact(deps)
	return deps
}

const StepActionCreateCertificate = "CreateCertificate"

type CreateCertificateStep struct {
	StepMeta        `json:",inline"`
	VaultBaseUrl    Value `json:"vaultBaseUrl,omitempty"`
	CertificateName Value `json:"certificateName,omitempty"`
	ContentType     Value `json:"contentType,omitempty"`
	SAN             Value `json:"san,omitempty"`
	Issuer          Value `json:"issuer,omitempty"`
	SecretKeyVault  Value `json:"secretKeyVault,omitempty"`
	SecretName      Value `json:"secretName,omitempty"`
	ApplicationId   Value `json:"applicationId,omitempty"`
}

func (s *CreateCertificateStep) Description() string {
	return fmt.Sprintf("Step %s\n Kind: %s\n", s.Name, s.Action)
}

func (s *CreateCertificateStep) RequiredInputs() []StepDependency {
	var deps []StepDependency
	for _, val := range []Value{s.VaultBaseUrl, s.CertificateName, s.ContentType, s.SAN, s.Issuer, s.SecretKeyVault, s.SecretName, s.ApplicationId} {
		if val.Input != nil {
			deps = append(deps, val.Input.StepDependency)
		}
	}
	slices.SortFunc(deps, SortDependencies)
	deps = slices.Compact(deps)
	return deps
}

const StepActionResourceProviderRegistration = "ResourceProviderRegistration"

type ResourceProviderRegistrationStep struct {
	StepMeta                   `json:",inline"`
	ResourceProviderNamespaces Value `json:"resourceProviderNamespaces,omitempty"`
}

func (s *ResourceProviderRegistrationStep) Description() string {
	return fmt.Sprintf("Step %s\n Kind: %s\n", s.Name, s.Action)
}

func (s *ResourceProviderRegistrationStep) RequiredInputs() []StepDependency {
	var deps []StepDependency
	for _, val := range []Value{s.ResourceProviderNamespaces} {
		if val.Input != nil {
			deps = append(deps, val.Input.StepDependency)
		}
	}
	slices.SortFunc(deps, SortDependencies)
	deps = slices.Compact(deps)
	return deps
}

const (
	StepActionRPLogs      = "RPLogsAccount"
	StepActionClusterLogs = "ClusterLogsAccount"
)

type LogsStep struct {
	StepMeta             `json:",inline"`
	RolloutKind          string            `json:"rolloutKind,omitempty"`
	TypeName             Value             `json:"typeName"`
	SecretKeyVault       Value             `json:"secretKeyVault,omitempty"`
	SecretName           Value             `json:"secretName,omitempty"`
	Environment          Value             `json:"environment"`
	AccountName          Value             `json:"accountName"`
	MetricsAccount       Value             `json:"metricsAccount"`
	AdminAlias           Value             `json:"adminAlias"`
	AdminGroup           Value             `json:"adminGroup"`
	SubscriptionId       Value             `json:"subscriptionId,omitempty"`
	Namespace            Value             `json:"namespace,omitempty"`
	CertSAN              Value             `json:"certsan,omitempty"`
	CertDescription      Value             `json:"certdescription,omitempty"`
	ConfigVersion        Value             `json:"configVersion,omitempty"`
	MonikerDefaultRegion Value             `json:"monikerDefaultRegion,omitempty"`
	Database             Value             `json:"database,omitempty"`
	Events               map[string]string `json:"events,omitempty"`
}

func (s *LogsStep) Description() string {
	return fmt.Sprintf("Step %s\n Kind: %s\n RolloutKind: %s\n", s.Name, s.Action, s.RolloutKind)
}

func (s *LogsStep) RequiredInputs() []StepDependency {
	var deps []StepDependency
	for _, val := range []Value{s.TypeName, s.SecretKeyVault, s.SecretName, s.Environment, s.AccountName, s.MetricsAccount, s.AdminAlias, s.AdminGroup, s.SubscriptionId, s.Namespace, s.CertSAN, s.CertDescription, s.ConfigVersion, s.MonikerDefaultRegion, s.Database} {
		if val.Input != nil {
			deps = append(deps, val.Input.StepDependency)
		}
	}
	slices.SortFunc(deps, SortDependencies)
	deps = slices.Compact(deps)
	return deps
}

const StepActionFeatureRegistration = "FeatureRegistration"

type FeatureRegistrationStep struct {
	StepMeta          `json:",inline"`
	SecretKeyVault    Value  `json:"secretKeyVault,omitempty"`
	SecretName        Value  `json:"secretName,omitempty"`
	ProviderConfigRef string `json:"providerConfigRef,omitempty"`
}

func (s *FeatureRegistrationStep) Description() string {
	return fmt.Sprintf("Step %s\n Kind: %s\n", s.Name, s.Action)
}

func (s *FeatureRegistrationStep) RequiredInputs() []StepDependency {
	var deps []StepDependency
	for _, val := range []Value{s.SecretKeyVault, s.SecretName} {
		if val.Input != nil {
			deps = append(deps, val.Input.StepDependency)
		}
	}
	slices.SortFunc(deps, SortDependencies)
	deps = slices.Compact(deps)
	return deps
}

const StepActionProviderFeatureRegistration = "ProviderFeatureRegistration"

type ProviderFeatureRegistrationStep struct {
	StepMeta          `json:",inline"`
	ProviderConfigRef string `json:"providerConfigRef,omitempty"`
	IdentityFrom      Input  `json:"identityFrom,omitempty"`
}

func (s *ProviderFeatureRegistrationStep) Description() string {
	return fmt.Sprintf("Step %s\n Kind: %s\n", s.Name, s.Action)
}

func (s *ProviderFeatureRegistrationStep) RequiredInputs() []StepDependency {
	return []StepDependency{s.IdentityFrom.StepDependency}
}

const StepActionEv2Registration = "Ev2Registration"

type Ev2RegistrationStep struct {
	StepMeta     `json:",inline"`
	IdentityFrom Input `json:"identityFrom,omitempty"`
}

func (s *Ev2RegistrationStep) Description() string {
	return fmt.Sprintf("Step %s\n Kind: %s\n", s.Name, s.Action)
}

func (s *Ev2RegistrationStep) RequiredInputs() []StepDependency {
	return []StepDependency{s.IdentityFrom.StepDependency}
}

const StepActionSecretSync = "SecretSync"

type SecretSyncStep struct {
	StepMeta          `json:",inline"`
	ConfigurationFile string `json:"configurationFile,omitempty"`
	KeyVault          string `json:"keyVault,omitempty"`
	EncryptionKey     string `json:"encryptionKey,omitempty"`
	IdentityFrom      Input  `json:"identityFrom,omitempty"`
}

func (s *SecretSyncStep) Description() string {
	return fmt.Sprintf("Step %s\n Kind: %s\n", s.Name, s.Action)
}

func (s *SecretSyncStep) RequiredInputs() []StepDependency {
	return []StepDependency{s.IdentityFrom.StepDependency}
}

const StepActionKusto = "Kusto"

type KustoStep struct {
	StepMeta         `json:",inline"`
	SecretKeyVault   Value `json:"secretKeyVault,omitempty"`
	SecretName       Value `json:"secretName,omitempty"`
	ApplicationId    Value `json:"applicationId,omitempty"`
	ConnectionString Value `json:"connectionString,omitempty"`
	Command          Value `json:"command,omitempty"`
}

func (s *KustoStep) Description() string {
	return fmt.Sprintf("Step %s\n Kind: %s\n", s.Name, s.Action)
}

func (s *KustoStep) RequiredInputs() []StepDependency {
	var deps []StepDependency
	for _, val := range []Value{s.SecretKeyVault, s.SecretName, s.ApplicationId, s.ConnectionString, s.Command} {
		if val.Input != nil {
			deps = append(deps, val.Input.StepDependency)
		}
	}
	slices.SortFunc(deps, SortDependencies)
	deps = slices.Compact(deps)
	return deps
}

const StepActionPav2 = "Pav2"

type Pav2Step struct {
	StepMeta                   `json:",inline"`
	SecretKeyVault             Value `json:"secretKeyVault,omitempty"`
	SecretName                 Value `json:"secretName,omitempty"`
	StorageAccount             Value `json:"storageAccount,omitempty"`
	SMEEndpointSuffixParameter Value `json:"smeEndpointSuffixParameter,omitempty"`
	SMEAppidParameter          Value `json:"smeAppidParameter,omitempty"`
}

func (s *Pav2Step) Description() string {
	return fmt.Sprintf("Step %s\n Kind: %s\n", s.Name, s.Action)
}

func (s *Pav2Step) RequiredInputs() []StepDependency {
	var deps []StepDependency
	for _, val := range []Value{s.SecretKeyVault, s.SecretName, s.StorageAccount, s.SMEEndpointSuffixParameter, s.SMEAppidParameter} {
		if val.Input != nil {
			deps = append(deps, val.Input.StepDependency)
		}
	}
	slices.SortFunc(deps, SortDependencies)
	deps = slices.Compact(deps)
	return deps
}
