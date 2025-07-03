package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Azure/ARO-Tools/pkg/config/types"
)

func TestValidateSchema(t *testing.T) {
	provider, err := NewConfigProvider("./testdata/config.yaml")
	require.NoError(t, err)

	resolver, err := provider.GetResolver(&ConfigReplacements{
		CloudReplacement:       "public",
		EnvironmentReplacement: "int",
		RegionReplacement:      "uksouth",
		RegionShortReplacement: "ln",
	})
	require.NoError(t, err)

	cfg, err := resolver.GetRegionConfiguration("uksouth")
	require.NoError(t, err)

	validationErr := resolver.ValidateSchema(cfg)
	require.NoError(t, validationErr)
}

func TestConvertToInterface(t *testing.T) {
	vars := types.Configuration{
		"key1": "value1",
		"key2": types.Configuration{
			"key3": "value3",
		},
	}

	expected := map[string]any{
		"key1": "value1",
		"key2": map[string]any{
			"key3": "value3",
		},
	}

	result := convertToInterface(vars)
	assert.Equal(t, expected, result)
	assert.IsType(t, expected, map[string]any{})
	assert.IsType(t, expected["key2"], map[string]any{})
}

func TestNormalizeConfigurationOverrides(t *testing.T) {
	testCases := []struct {
		name string
		cfg  *configurationOverrides
	}{
		{
			name: "nested maps with deep structure",
			cfg: &configurationOverrides{
				Schema: "schema.json",
				Defaults: types.Configuration{
					"service": map[string]interface{}{
						"name": "default-service",
					},
					"geneva": map[string]interface{}{
						"logs": map[string]interface{}{
							"administrators": map[string]interface{}{
								"alias": "default-alias",
							},
						},
					},
				},
				Overrides: map[string]*struct {
					Defaults  types.Configuration `json:"defaults"`
					Overrides map[string]*struct {
						Defaults  types.Configuration            `json:"defaults"`
						Overrides map[string]types.Configuration `json:"regions"`
					} `json:"environments"`
				}{
					"public": {
						Defaults: types.Configuration{
							"service": map[string]interface{}{
								"cloud": "public",
							},
						},
						Overrides: map[string]*struct {
							Defaults  types.Configuration            `json:"defaults"`
							Overrides map[string]types.Configuration `json:"regions"`
						}{
							"int": {
								Defaults: types.Configuration{
									"service": map[string]interface{}{
										"env": "int",
									},
								},
								Overrides: map[string]types.Configuration{
									"uksouth": {
										"service": map[string]interface{}{
											"region": "uksouth",
										},
										"geneva": map[string]interface{}{
											"logs": map[string]interface{}{
												"rp": map[string]interface{}{
													"accountName": "uksouth-account",
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
		{
			name: "nil values at various levels",
			cfg: &configurationOverrides{
				Defaults: types.Configuration{
					"key": "value",
				},
				Overrides: map[string]*struct {
					Defaults  types.Configuration `json:"defaults"`
					Overrides map[string]*struct {
						Defaults  types.Configuration            `json:"defaults"`
						Overrides map[string]types.Configuration `json:"regions"`
					} `json:"environments"`
				}{
					"cloud1": nil,
					"cloud2": {
						Defaults: types.Configuration{},
						Overrides: map[string]*struct {
							Defaults  types.Configuration            `json:"defaults"`
							Overrides map[string]types.Configuration `json:"regions"`
						}{
							"env1": nil,
							"env2": {
								Defaults:  types.Configuration{},
								Overrides: nil,
							},
						},
					},
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Define inline helper function
			var verifyNoMapStringInterface func(path string, v interface{}, inconsistentPaths *[]string)
			verifyNoMapStringInterface = func(path string, v interface{}, inconsistentPaths *[]string) {
				switch val := v.(type) {
				case types.Configuration:
					for k, nested := range val {
						verifyNoMapStringInterface(path+"."+k, nested, inconsistentPaths)
					}
				case map[string]interface{}:
					*inconsistentPaths = append(*inconsistentPaths, path)
					for k, nested := range val {
						verifyNoMapStringInterface(path+"."+k, nested, inconsistentPaths)
					}
				}
			}

			normalizeConfigurationOverrides(tc.cfg)

			// Verify that all nested maps are of type types.Configuration
			inconsistentPaths := []string{}

			verifyNoMapStringInterface("defaults", tc.cfg.Defaults, &inconsistentPaths)
			for cloudName, cloudCfg := range tc.cfg.Overrides {
				if cloudCfg != nil {
					verifyNoMapStringInterface("overrides."+cloudName+".defaults", cloudCfg.Defaults, &inconsistentPaths)

					for envName, envCfg := range cloudCfg.Overrides {
						if envCfg != nil {
							verifyNoMapStringInterface("overrides."+cloudName+"."+envName+".defaults", envCfg.Defaults, &inconsistentPaths)

							for regionName, regionCfg := range envCfg.Overrides {
								verifyNoMapStringInterface("overrides."+cloudName+"."+envName+"."+regionName, regionCfg, &inconsistentPaths)
							}
						}
					}
				}
			}

			assert.Empty(t, inconsistentPaths, "Found map[string]interface{} at paths: %v (should be types.Configuration)", inconsistentPaths)
		})
	}
}
