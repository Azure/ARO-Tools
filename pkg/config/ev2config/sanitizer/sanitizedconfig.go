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
}

type KeyVaultValues struct {
	DomainNameSuffix string `json:"domainNameSuffix"`
}

type SanitizedRegionConfig struct {
	AvailabilityZoneCount int    `json:"availabilityZoneCount"`
	RegionShortName       string `json:"regionShortName"`
	RegionFriendlyName    string `json:"regionFriendlyName"`
}

type AzureContainerRegistryValues struct {
	DomainNameSuffix string `json:"domainNameSuffix"`
}
