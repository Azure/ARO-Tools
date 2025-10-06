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

// WellFormedChecker allows introspection of how well-formed this step is over inputs.
type WellFormedChecker interface {
	// IsWellFormedOverInputs determines if, given the same (visible) inputs, this step will have the same outputs.
	// Implicit inputs on the filesystem outside the purview of any configuration make a step ill-formed.
	IsWellFormedOverInputs() bool
}

// Step divulges common metadata about a step.
type Step interface {
	WellFormedChecker
	StepName() string
	ActionType() string
	Description() string
	Dependencies() []StepDependency
	RequiredInputs() []StepDependency
	ExternalDependencies() []ExternalStepDependency
	AutomatedRetries() *AutomatedRetry
}

type ValidationStep interface {
	Step
	Validations() []string
}

// StepMeta contains metadata for a steps.
type StepMeta struct {
	Name              string                   `json:"name"`
	Action            string                   `json:"action"`
	AutomatedRetry    *AutomatedRetry          `json:"automatedRetry,omitempty"`
	DependsOn         []StepDependency         `json:"dependsOn,omitempty"`
	ExternalDependsOn []ExternalStepDependency `json:"externalDependsOn,omitempty"`
}

// StepDependency describes a step that must run before the dependent step may begin.
type StepDependency struct {
	// ResourceGroup is the (semantic/display) name of the group to which the step belongs.
	ResourceGroup string `json:"resourceGroup"`
	// Step is the name of the step being depended on.
	Step string `json:"step"`
}

type ExternalStepDependency struct {
	// ServiceGroup declares which service group the external dependency belongs to.
	ServiceGroup   string `json:"serviceGroup"`
	StepDependency `json:",inline"`
}

// AutomatedRetry configures automated retry for failed steps.
type AutomatedRetry struct {
	// ErrorContainsAny determines when a retry should run - if the output of a step contains any of
	// the strings in this array, matching in a case-insensitive manner, a retry will fire.
	// Must contain 16 or fewer items; the total encoded length of this array may not be more than 1KB.
	ErrorContainsAny []string `json:"errorContainsAny,omitempty"`

	// MaximumRetryCount is the maximum number of retires that should fire. Between 1 and 10, defaults to 1.
	MaximumRetryCount int `json:"maximumRetryCount,omitempty"`

	// DurationBetweenRetries is the amount of time to wait between retries. Must be between 1 minute and 3 hours.
	// Formatted using Go's time.Duration syntax.
	DurationBetweenRetries string `json:"durationBetweenRetries,omitempty"`
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

func (m *StepMeta) AutomatedRetries() *AutomatedRetry {
	return m.AutomatedRetry
}

// Dependencies exposes the dependencies this step has on other steps for the same service group.
func (m *StepMeta) Dependencies() []StepDependency {
	return m.DependsOn
}

// ExternalDependencies exposes the dependencies this step has on steps in *other* service groups. When provided,
// this will add to the default behavior of depending on "all" leaf outputs from the parent service group as
// configured in the topology. Be careful when using this to encode intent directly. Depending on steps from many
// service groups is supported. In single-service-group contexts, these dependencies are ignored.
func (m *StepMeta) ExternalDependencies() []ExternalStepDependency {
	return m.ExternalDependsOn
}

func (m *StepMeta) IsWellFormedOverInputs() bool {
	return true
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

func (s *GenericStep) IsWellFormedOverInputs() bool {
	return false
}

type GenericValidationStep struct {
	StepMeta   `json:",inline"`
	Validation []string `json:"validation,omitempty"`
}

func (s *GenericValidationStep) Description() string {
	return fmt.Sprintf("Step %s\n  Kind: %s", s.Name, s.Action)
}

func (s *GenericValidationStep) RequiredInputs() []StepDependency {
	return []StepDependency{}
}

func (s *GenericValidationStep) Validations() []string {
	return s.Validation
}

func (s *GenericValidationStep) IsWellFormedOverInputs() bool {
	return false
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
	CommonName      Value `json:"commonName,omitempty"`
}

func (s *CreateCertificateStep) Description() string {
	return fmt.Sprintf("Step %s\n Kind: %s\n", s.Name, s.Action)
}

func (s *CreateCertificateStep) RequiredInputs() []StepDependency {
	var deps []StepDependency
	for _, val := range []Value{s.VaultBaseUrl, s.CertificateName, s.ContentType, s.SAN, s.Issuer, s.SecretKeyVault, s.SecretName, s.ApplicationId, s.CommonName} {
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
	SecretKeyVault             Value  `json:"secretKeyVault,omitempty"`
	SecretName                 Value  `json:"secretName,omitempty"`
	StorageAccount             Value  `json:"storageAccount,omitempty"`
	SMEEndpointSuffixParameter Value  `json:"smeEndpointSuffixParameter,omitempty"`
	SMEAppidParameter          Value  `json:"smeAppidParameter,omitempty"`
	Operation                  string `json:"operation,omitempty"`
}

func (s *Pav2Step) Description() string {
	return fmt.Sprintf("Step %s\n Kind: %s\n Operation: %s\n", s.Name, s.Action, s.Operation)
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

const StepActionHelm = "Helm"

type HelmStep struct {
	StepMeta `json:",inline"`

	// AKSCluster is the name of the AKS cluster onto which this Helm release will be deployed.
	AKSCluster string `json:"aksCluster"`
	// SubnetName is an optional specifier for the name of the subnet to which the deployer must connect to before
	// accessing the AKS cluster. Provide this value when deploying to private clusters.
	SubnetName string `json:"subnetName,omitempty"`

	// ReleaseName is the semantically-meaningful name of the Helm release. The first deployment for a given name
	// will install the Helm chart, further deployments to the same name will upgrade it.
	ReleaseName string `json:"releaseName"`
	// ReleaseNamespace is the name of the namespace in which the Helm release should be deployed, analogous to the
	// --namespace flag for the Helm CLI. This namespace will be created before the Helm release is installed.
	ReleaseNamespace string `json:"releaseNamespace"`
	// NamespaceFiles specify namespaces that must be created before the Helm release is installed. It is *not* required
	// to specify the release namespace manifest here, but it may be present if a complex configuration (with labels,
	// annotations, spec, etc.) is required. By default, the release namespace will be created with no additional fields
	// set if no additional manifests are specified in this field.
	// NOTE: These files will be pre-processed as Go templates to resolve configuration fields and input variables.
	NamespaceFiles []string `json:"namespaceFiles,omitempty"`
	// ChartDir is the relative path from the pipeline configuration to the chart being deployed.
	ChartDir string `json:"chartDir"`
	// ValuesFile is the path to the Helm values file to use when deploying the Helm release.
	// NOTE: This file will be pre-processed as a Go template to resolve configuration fields and input variables.
	ValuesFile string `json:"valuesFile,omitempty"`

	// InputVariables records a mapping from variable names to the output variable that provides the value.
	// For some input variable like:
	//     inputVariables:
	//       someImportantThing:
	//         resourceGroup: regional
	//         step: output
	//         name: outputVariableName
	// Refer to this value in the namespace files or values.yaml with __someImportantThing__.
	InputVariables map[string]Input `json:"inputVariables,omitempty"`

	// IdentityFrom specifies the managed identity with which this deployment will run in Ev2.
	IdentityFrom Input `json:"identityFrom,omitempty"`
}

func (s *HelmStep) Description() string {
	return fmt.Sprintf("Step %s\n Kind: %s\n", s.Name, s.Action)
}

func (s *HelmStep) RequiredInputs() []StepDependency {
	deps := []StepDependency{s.IdentityFrom.StepDependency}
	for _, val := range s.InputVariables {
		deps = append(deps, val.StepDependency)
	}
	slices.SortFunc(deps, SortDependencies)
	deps = slices.Compact(deps)
	return deps
}
