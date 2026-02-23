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

	"github.com/Azure/ARO-Tools/config"
	"github.com/Azure/ARO-Tools/config/ev2config"
	"github.com/Azure/ARO-Tools/testutil"
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

func TestSubscriptionProvisioningSchemaValidation(t *testing.T) {
	tests := []struct {
		name        string
		yaml        string
		expectError bool
		description string
	}{
		{
			name: "backfill_only",
			yaml: `
$schema: pipeline.schema.v1
serviceGroup: Microsoft.Azure.ARO.Test
rolloutName: Test Rollout
resourceGroups:
- name: test
  resourceGroup: test-rg
  subscription: test-sub
  subscriptionProvisioning:
    backfillSubscriptionId:
      value: existing-sub-123
  steps:
  - name: deploy
    action: Shell
    command: echo hello
`,
			expectError: false,
			description: "backfillSubscriptionId alone should be valid",
		},
		{
			name: "new_subscription_only",
			yaml: `
$schema: pipeline.schema.v1
serviceGroup: Microsoft.Azure.ARO.Test
rolloutName: Test Rollout
resourceGroups:
- name: test
  resourceGroup: test-rg
  subscription: test-sub
  subscriptionProvisioning:
    displayName:
      value: "New Subscription"
    roleAssignment: param/foo.bicepparam
  steps:
  - name: deploy
    action: Shell
    command: echo hello
`,
			expectError: false,
			description: "displayName and roleAssignment alone should be valid",
		},
		{
			name: "backfill_with_display_name",
			yaml: `
$schema: pipeline.schema.v1
serviceGroup: Microsoft.Azure.ARO.Test
rolloutName: Test Rollout
resourceGroups:
- name: test
  resourceGroup: test-rg
  subscription: test-sub
  subscriptionProvisioning:
    backfillSubscriptionId:
      value: existing-sub-123
    displayName:
      value: "My Subscription"
  steps:
  - name: deploy
    action: Shell
    command: echo hello
`,
			expectError: false,
			description: "backfillSubscriptionId with displayName should be valid (new behavior)",
		},
		{
			name: "backfill_with_all_fields",
			yaml: `
$schema: pipeline.schema.v1
serviceGroup: Microsoft.Azure.ARO.Test
rolloutName: Test Rollout
resourceGroups:
- name: test
  resourceGroup: test-rg
  subscription: test-sub
  subscriptionProvisioning:
    backfillSubscriptionId:
      value: existing-sub-123
    displayName:
      value: "My Subscription"
    roleAssignment: param/foo.bicepparam
    airsRegisteredUserPrincipalId:
      value: "user-123"
    certificateDomains:
      value: "*.example.com"
  steps:
  - name: deploy
    action: Shell
    command: echo hello
`,
			expectError: false,
			description: "backfillSubscriptionId with all other fields should be valid (new behavior)",
		},
		{
			name: "missing_required_fields",
			yaml: `
$schema: pipeline.schema.v1
serviceGroup: Microsoft.Azure.ARO.Test
rolloutName: Test Rollout
resourceGroups:
- name: test
  resourceGroup: test-rg
  subscription: test-sub
  subscriptionProvisioning:
    airsRegisteredUserPrincipalId:
      value: "user-123"
  steps:
  - name: deploy
    action: Shell
    command: echo hello
`,
			expectError: true,
			description: "missing both required patterns should fail validation",
		},
		{
			name: "display_name_without_role_assignment",
			yaml: `
$schema: pipeline.schema.v1
serviceGroup: Microsoft.Azure.ARO.Test
rolloutName: Test Rollout
resourceGroups:
- name: test
  resourceGroup: test-rg
  subscription: test-sub
  subscriptionProvisioning:
    displayName:
      value: "New Subscription"
  steps:
  - name: deploy
    action: Shell
    command: echo hello
`,
			expectError: true,
			description: "displayName without roleAssignment should fail validation",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
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
			require.NoError(t, err)

			_, err = NewPipelineFromBytes([]byte(tt.yaml), cfg)
			if tt.expectError {
				assert.Error(t, err, tt.description)
			} else {
				assert.NoError(t, err, tt.description)
			}
		})
	}
}
