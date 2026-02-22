package config

import (
	"testing"

	"github.com/stretchr/testify/require"
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
