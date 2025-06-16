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
