package main

func Sanitize(inputs map[string]CentralConfig) SanitizedConfig {
	output := SanitizedConfig{
		Clouds: map[string]SanitizedCloudConfig{},
	}
	for cloud, cfg := range inputs {
		regions := map[string]SanitizedRegionConfig{}
		for _, geo := range cfg.Geographies {
			for _, region := range geo.Regions {
				regions[region.Name] = SanitizedRegionConfig{
					AvailabilityZoneCount: region.Settings.AvailabilityZoneCount,
					RegionShortName:       region.Settings.RegionShortName,
					RegionFriendlyName:    region.Settings.RegionFriendlyName,
				}
			}
		}
		output.Clouds[cloud] = SanitizedCloudConfig{
			Defaults: SanitizedCloudConfigValues{
				CloudName: cfg.Settings.CloudName,
				KeyVault: KeyVaultValues{
					DomainNameSuffix: cfg.Settings.KeyVault.DomainNameSuffix,
				},
				AzureContainerRegistry: AzureContainerRegistryValues{
					DomainNameSuffix: cfg.Settings.AzureContainerRegistry.DomainNameSuffix,
				},
			},
			Regions: regions,
		}
	}
	return output
}
