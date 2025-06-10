package main

type SanitizedConfig struct {
	Clouds map[string]SanitizedCloudConfig `json:"clouds"`
}

type SanitizedCloudConfig struct {
	Defaults SanitizedCloudConfigValues       `json:"defaults"`
	Environments  map[string]SanitizedEnvironmentConfig `json:"environments"`
}

type SanitizedEnvironmentConfig struct {
	Regions  map[string]SanitizedRegionConfig `json:"regions"`
}

type SanitizedCloudConfigValues struct {
	KeyVault KeyVaultValues `json:"keyVault"`
}

type KeyVaultValues struct {
	DomainNameSuffix string `json:"domainNameSuffix"`
}

type SanitizedRegionConfig struct {
	AvailabilityZoneCount int    `json:"availabilityZoneCount"`
	RegionShortName       string `json:"regionShortName"`
}
