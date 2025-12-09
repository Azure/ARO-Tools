package config

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Azure/ARO-Tools/pkg/config/ev2config"
)

func TestValidateSchema(t *testing.T) {
	provider, err := NewConfigProvider("./testdata/config.yaml")
	require.NoError(t, err)

	ev2Cfg, err := ev2config.ResolveConfig("public", "uksouth")
	require.NoError(t, err)

	resolver, err := provider.GetResolver(&ConfigReplacements{
		CloudReplacement:       "public",
		EnvironmentReplacement: "int",
		RegionReplacement:      "uksouth",
		RegionShortReplacement: "ln",
		Ev2Config:              ev2Cfg,
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

	// Verify the error is the correct type and contains expected collision paths
	var collisionErr *SchemaCollisionError
	require.ErrorAs(t, err, &collisionErr)
	require.Contains(t, collisionErr.SchemaPath, "config.schema.with-ev2-conflicts.json")
	require.ElementsMatch(t, []string{"geoShortId", "geneva.actions.homeDsts.primary", "entra.fqdn"}, collisionErr.Collisions)
}
