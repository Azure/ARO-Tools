// Copyright 2026 Microsoft Corporation
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

package manage

import (
	"context"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Azure/ARO-Tools/tools/grafanactl/cmd/base"
)

func TestDefaultReconcileOptions(t *testing.T) {
	opts := DefaultReconcileOptions()

	if opts.SKU != "Standard" {
		t.Fatalf("expected default SKU 'Standard', got %q", opts.SKU)
	}
	if opts.ZoneRedundancy != "Disabled" {
		t.Fatalf("expected default ZoneRedundancy 'Disabled', got %q", opts.ZoneRedundancy)
	}
	if opts.PublicNetworkAccess != "Enabled" {
		t.Fatalf("expected default PublicNetworkAccess 'Enabled', got %q", opts.PublicNetworkAccess)
	}
}

func TestValidatePublicNetworkAccess(t *testing.T) {
	for _, tc := range []struct {
		name                string
		publicNetworkAccess string
		wantErrSub          string
	}{
		{
			name:                "Enabled is valid",
			publicNetworkAccess: "Enabled",
		},
		{
			name:                "Disabled is valid",
			publicNetworkAccess: "Disabled",
		},
		{
			name:                "empty string is rejected",
			publicNetworkAccess: "",
			wantErrSub:          "--public-network-access must be",
		},
		{
			name:                "invalid value is rejected",
			publicNetworkAccess: "Invalid",
			wantErrSub:          "--public-network-access must be",
		},
		{
			name:                "lowercase is rejected",
			publicNetworkAccess: "enabled",
			wantErrSub:          "--public-network-access must be",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			opts := DefaultReconcileOptions()
			opts.Location = "eastus"
			opts.GrafanaName = "test-grafana"
			opts.SubscriptionID = "00000000-0000-0000-0000-000000000000"
			opts.ResourceGroup = "test-rg"
			opts.PublicNetworkAccess = tc.publicNetworkAccess

			_, err := opts.Validate(context.Background())
			if tc.wantErrSub != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tc.wantErrSub)
				}
				if !strings.Contains(err.Error(), tc.wantErrSub) {
					t.Fatalf("expected error containing %q, got %q", tc.wantErrSub, err.Error())
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestBindReconcileOptionsADXFlags(t *testing.T) {
	options := DefaultReconcileOptions()
	command := &cobra.Command{Use: "reconcile"}
	require.NoError(t, BindReconcileOptions(options, command))

	require.NoError(t, command.ParseFlags([]string{
		"--adx-integrations-enabled",
		"--adx-integrations-environment=int-one",
		"--adx-integrations-geographies=EUS,WUS",
		"--adx-integrations-scenario=scenario",
		"--adx-integrations-target-resource-id=/subscriptions/example/resourceGroups/rg/providers/example/type/name",
	}))

	assert.True(t, options.ADXIntegrationsEnabled)
	assert.Equal(t, "int-one", options.ADXEnvironment)
	assert.Equal(t, "EUS,WUS", options.ADXGeographies)
	assert.Equal(t, "scenario", options.ADXScenario)
	assert.Equal(t, "/subscriptions/example/resourceGroups/rg/providers/example/type/name", options.ADXTargetResourceID)
}

func TestValidateADXOptions(t *testing.T) {
	newOptions := func() *RawReconcileOptions {
		return &RawReconcileOptions{
			BaseOptions: &base.BaseOptions{
				SubscriptionID: "subscription-id",
				ResourceGroup:  "resource-group",
				GrafanaName:    "grafana",
				OutputFormat:   "table",
			},
			Location:            "eastus",
			SKU:                 "Standard",
			ZoneRedundancy:      "Disabled",
			PublicNetworkAccess: "Enabled",
		}
	}

	t.Run("disabled does not require ADX selection", func(t *testing.T) {
		_, err := newOptions().Validate(t.Context())
		require.NoError(t, err)
	})

	t.Run("enabled requires environment", func(t *testing.T) {
		options := newOptions()
		options.ADXIntegrationsEnabled = true
		options.ADXGeographies = "eus"
		_, err := options.Validate(t.Context())
		assert.ErrorContains(t, err, "--adx-integrations-environment is required")
	})

	t.Run("enabled rejects unsafe environment", func(t *testing.T) {
		options := newOptions()
		options.ADXIntegrationsEnabled = true
		options.ADXEnvironment = "int'bad"
		options.ADXGeographies = "eus"
		_, err := options.Validate(t.Context())
		assert.ErrorContains(t, err, "only ASCII letters, numbers, and hyphens")
	})

	t.Run("enabled requires geographies", func(t *testing.T) {
		options := newOptions()
		options.ADXIntegrationsEnabled = true
		options.ADXEnvironment = "int"
		_, err := options.Validate(t.Context())
		assert.ErrorContains(t, err, "--adx-integrations-geographies is required")
	})

	t.Run("enabled accepts explicit selection", func(t *testing.T) {
		options := newOptions()
		options.ADXIntegrationsEnabled = true
		options.ADXEnvironment = "int-one"
		options.ADXGeographies = "eus,wus"
		_, err := options.Validate(t.Context())
		require.NoError(t, err)
	})
}
