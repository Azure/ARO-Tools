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

package types

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Azure/ARO-Tools/internal/testutil"
	"github.com/Azure/ARO-Tools/pkg/config"
	"github.com/Azure/ARO-Tools/pkg/config/ev2config"
)

func TestNewPipelineFromFile(t *testing.T) {
	region := "uksouth"
	regionShort := "uks"
	stamp := "1"
	cloud := "public"
	environment := "int"

	ev2, err := ev2config.ResolveConfig(cloud, region)
	require.NoError(t, err)
	provider, err := config.NewConfigProvider("../../testdata/config.yaml")
	require.NoError(t, err)
	resolver, err := provider.GetResolver(&config.ConfigReplacements{
		RegionReplacement:      region,
		RegionShortReplacement: regionShort,
		StampReplacement:       stamp,
		CloudReplacement:       cloud,
		EnvironmentReplacement: environment,
		Ev2Config:              ev2,
	})
	require.NoError(t, err)

	cfg, err := resolver.GetRegionConfiguration(region)
	assert.NoError(t, err)

	pipeline, err := NewPipelineFromFile("../../testdata/pipeline.yaml", cfg)
	assert.NoError(t, err)

	testutil.CompareWithFixture(t, pipeline, testutil.WithExtension(".yaml"))
}
