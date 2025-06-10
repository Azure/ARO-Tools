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
				}
			}
		}
		output.Clouds[cloud] = SanitizedCloudConfig{
			Defaults: SanitizedCloudConfigValues{
				KeyVault: KeyVaultValues{
					DomainNameSuffix: cfg.Settings.KeyVault.DomainNameSuffix,
				},
			},
			Environments: map[string]SanitizedEnvironmentConfig{
				"prod": { // we never have differences per environment, but need this level of nesting to match the core config
					Regions: regions,
				},
			},
		}
	}
	return output
}
