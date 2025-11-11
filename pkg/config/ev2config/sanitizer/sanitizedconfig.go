package main

type SanitizedConfig struct {
	Clouds map[string]SanitizedCloudConfig `json:"clouds"`
}

type SanitizedCloudConfig struct {
	Defaults SanitizedCloudConfigValues       `json:"defaults"`
	Regions  map[string]SanitizedRegionConfig `json:"regions"`
}

type SanitizedCloudConfigValues struct {
	CloudName              string                       `json:"cloudName"`
	KeyVault               KeyVaultValues               `json:"keyVault"`
	AzureContainerRegistry AzureContainerRegistryValues `json:"azureContainerRegistry"`
	Entra                  SanitizedEntraConfig         `json:"entra"`
	ARM                    SanitizedARMConfig           `json:"arm"`
	Geneva                 SanitizedGenevaConfig        `json:"geneva"`
}

type KeyVaultValues struct {
	DomainNameSuffix string `json:"domainNameSuffix"`
}

type SanitizedEntraConfig struct {
	FederatedCredentials EntraFederatedCredentials `json:"federatedcredentials"`
	FQDN                 map[string]string         `json:"fqdn"`
	Tenants              map[string]EntraTenant    `json:"tenants"`
}

type EntraFederatedCredentialValues struct {
	Audience string `json:"audience"`
}

type SanitizedARMConfig struct {
	Endpoint string `json:"endpoint"`
}

type SanitizedGenevaConfig struct {
	Actions SanitizedGenevaActionsConfig `json:"actions"`
}

type SanitizedGenevaActionsConfig struct {
	HomeDsts map[string]string `json:"homeDsts"`
}

type SanitizedRegionConfig struct {
	AvailabilityZoneCount int    `json:"availabilityZoneCount"`
	RegionShortName       string `json:"regionShortName"`
	RegionFriendlyName    string `json:"regionFriendlyName"`
}

type AzureContainerRegistryValues struct {
	DomainNameSuffix string `json:"domainNameSuffix"`
}
