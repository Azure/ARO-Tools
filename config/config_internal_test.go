// Copyright 2025 Microsoft Corporation
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package config

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValidateSchema(t *testing.T) {
	t.Parallel()
	provider, err := NewConfigProvider("./testdata/config.yaml")
	require.NoError(t, err)

	resolver, err := provider.GetResolver(&ConfigReplacements{
		CloudReplacement:       "public",
		EnvironmentReplacement: "int",
		RegionReplacement:      "uksouth",
		RegionShortReplacement: "ln",
	})
	require.NoError(t, err)

	cfg, err := resolver.GetRegionConfiguration("uksouth", "1")
	require.NoError(t, err)

	validationErr := resolver.ValidateSchema(cfg)
	require.NoError(t, validationErr)
}

func TestValidateSchemaWithCEL(t *testing.T) {
	t.Parallel()
	t.Run("valid config passes CEL rules", func(t *testing.T) {
		t.Parallel()
		provider, err := NewConfigProvider("./testdata/config-cel.yaml")
		require.NoError(t, err)

		resolver, err := provider.GetResolver(&ConfigReplacements{
			CloudReplacement:       "public",
			EnvironmentReplacement: "int",
			RegionReplacement:      "uksouth",
			RegionShortReplacement: "ln",
		})
		require.NoError(t, err)

		cfg, err := resolver.GetRegionConfiguration("uksouth", "1")
		require.NoError(t, err)

		validationErr := resolver.ValidateSchema(cfg)
		require.NoError(t, validationErr)
	})

	t.Run("invalid config fails CEL rules", func(t *testing.T) {
		t.Parallel()
		provider, err := NewConfigProvider("./testdata/config-cel-invalid.yaml")
		require.NoError(t, err)

		resolver, err := provider.GetResolver(&ConfigReplacements{
			CloudReplacement:       "public",
			EnvironmentReplacement: "int",
			RegionReplacement:      "uksouth",
			RegionShortReplacement: "ln",
		})
		require.NoError(t, err)

		cfg, err := resolver.GetRegionConfiguration("uksouth", "1")
		require.NoError(t, err)

		validationErr := resolver.ValidateSchema(cfg)
		require.Error(t, validationErr)
		require.Contains(t, validationErr.Error(), "key2 must be positive")
		require.Contains(t, validationErr.Error(), "version must be valid semver")
	})
}
