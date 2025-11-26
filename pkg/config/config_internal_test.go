package config

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Azure/ARO-Tools/pkg/config/ev2config"
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

func TestValidateEV2SchemaCollisionDetection(t *testing.T) {
	provider, err := NewConfigProvider("./testdata/config.with-ev2-conflicts.yaml")
	require.NoError(t, err)

	ev2Cfg, err := ev2config.ResolveConfig("public", "uksouth")
	require.NoError(t, err)

	// GetResolver should fail because the schema has a collision with geoShortId
	_, err = provider.GetResolver(&ConfigReplacements{
		CloudReplacement:       "public",
		EnvironmentReplacement: "int",
		RegionReplacement:      "uksouth",
		RegionShortReplacement: "ln",
		Ev2Config:              ev2Cfg,
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "collide with reserved ev2 config fields")
	require.Contains(t, err.Error(), "geoShortId")
	require.Contains(t, err.Error(), "config.schema.with-ev2-conflicts.json")
}
