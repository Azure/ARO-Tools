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

package modify

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/Azure/ARO-Tools/tools/cmdutils"
	"github.com/Azure/ARO-Tools/tools/grafanactl/cmd/base"
	"github.com/Azure/ARO-Tools/tools/grafanactl/internal/azure"
	"github.com/Azure/ARO-Tools/tools/grafanactl/internal/grafana"
)

// RawAddDatasourceOptions represents the initial, unvalidated configuration for add datasource operations.
type RawAddDatasourceOptions struct {
	*base.BaseOptions
	TagKey                string
	TagValue              string
	AzureMonitorEnabled   bool
	ADXEnabled            bool
	ADXDeleteWhenDisabled bool
	ADXClusterURL         string
	ADXDefaultDatabase    string
	ADXDatasourceName     string
	ADXGeographies        string
	ADXCurrentGeography   string
	ADXDataConsistency    string
}

// validatedAddDatasourceOptions is a private struct that enforces the options validation pattern.
type validatedAddDatasourceOptions struct {
	*RawAddDatasourceOptions
}

// ValidatedAddDatasourceOptions represents add datasource configuration that has passed validation.
type ValidatedAddDatasourceOptions struct {
	// Embed a private pointer that cannot be instantiated outside of this package
	*validatedAddDatasourceOptions
}

// CompletedAddDatasourceOptions represents the final, fully validated and initialized configuration
// for add datasource operations.
type CompletedAddDatasourceOptions struct {
	*validatedAddDatasourceOptions
	MonitorWorkspaceClient *azure.MonitorWorkspaceClient
	ManagedGrafanaClient   *azure.ManagedGrafanaClient
	GrafanaClient          adxGrafanaClient
}

// DefaultAddDatasourceOptions returns a new RawAddDatasourceOptions with default values
func DefaultAddDatasourceOptions() *RawAddDatasourceOptions {
	return &RawAddDatasourceOptions{
		BaseOptions:         base.DefaultBaseOptions(),
		TagKey:              "grafanactl-discovery",
		TagValue:            "true",
		AzureMonitorEnabled: true,
		ADXDataConsistency:  defaultADXDataConsistency,
	}
}

// BindAddDatasourceOptions binds command-line flags to the options
func BindAddDatasourceOptions(opts *RawAddDatasourceOptions, cmd *cobra.Command) error {
	if err := base.BindBaseOptions(opts.BaseOptions, cmd); err != nil {
		return err
	}

	flags := cmd.Flags()
	flags.StringVar(&opts.TagKey, "tag-key", opts.TagKey, "Azure Monitor Workspace tag key to filter by")
	flags.StringVar(&opts.TagValue, "tag-value", opts.TagValue, "Azure Monitor Workspace tag value to filter by")
	flags.BoolVar(&opts.AzureMonitorEnabled, "azure-monitor-enabled", opts.AzureMonitorEnabled, "Reconcile Azure Monitor Workspace integrations")
	flags.BoolVar(&opts.ADXEnabled, "adx-enabled", opts.ADXEnabled, "Reconcile the Azure Data Explorer datasource desired state as present")
	flags.BoolVar(&opts.ADXDeleteWhenDisabled, "adx-delete-when-disabled", opts.ADXDeleteWhenDisabled, "Delete the named Azure Data Explorer datasource when ADX desired state is disabled")
	flags.StringVar(&opts.ADXClusterURL, "adx-cluster-url", opts.ADXClusterURL, "Azure Data Explorer cluster URL")
	flags.StringVar(&opts.ADXDefaultDatabase, "adx-default-database", opts.ADXDefaultDatabase, "Default Azure Data Explorer database")
	flags.StringVar(&opts.ADXDatasourceName, "adx-datasource-name", opts.ADXDatasourceName, "Grafana Azure Data Explorer datasource name")
	flags.StringVar(&opts.ADXGeographies, "adx-geographies", opts.ADXGeographies, "Comma-separated geography short IDs where the Azure Data Explorer datasource should be present")
	flags.StringVar(&opts.ADXCurrentGeography, "adx-current-geography", opts.ADXCurrentGeography, "Current geography short ID used with --adx-geographies")
	flags.StringVar(&opts.ADXDataConsistency, "adx-data-consistency", opts.ADXDataConsistency, "Azure Data Explorer datasource data consistency")

	return nil
}

// Validate performs validation on the raw options
func (o *RawAddDatasourceOptions) Validate(ctx context.Context) (*ValidatedAddDatasourceOptions, error) {
	if err := base.ValidateBaseOptions(o.BaseOptions); err != nil {
		return nil, err
	}

	normalized := *o
	if strings.TrimSpace(normalized.ADXGeographies) != "" {
		allowed, err := adxGeographyAllowed(normalized.ADXGeographies, normalized.ADXCurrentGeography)
		if err != nil {
			return nil, err
		}
		if !allowed {
			normalized.ADXEnabled = false
		}
	}
	if normalized.ADXEnabled || normalized.ADXDeleteWhenDisabled {
		adxOptions, err := normalized.rawADXOptions().Validate(ctx)
		if err != nil {
			return nil, fmt.Errorf("invalid ADX datasource options: %w", err)
		}
		normalized.ADXEnabled = adxOptions.Enabled
		normalized.ADXDeleteWhenDisabled = adxOptions.DeleteWhenDisabled
		normalized.ADXClusterURL = adxOptions.ClusterURL
		normalized.ADXDefaultDatabase = adxOptions.DefaultDatabase
		normalized.ADXDatasourceName = adxOptions.DatasourceName
		normalized.ADXDataConsistency = adxOptions.DataConsistency
	}

	return &ValidatedAddDatasourceOptions{
		validatedAddDatasourceOptions: &validatedAddDatasourceOptions{
			RawAddDatasourceOptions: &normalized,
		},
	}, nil
}

func adxGeographyAllowed(geographies, currentGeography string) (bool, error) {
	currentGeography = strings.ToLower(strings.TrimSpace(currentGeography))
	if currentGeography == "" {
		return false, fmt.Errorf("adx-current-geography is required when adx-geographies is set")
	}
	if !isValidADXGeography(currentGeography) {
		return false, fmt.Errorf("adx-current-geography has invalid value %q", currentGeography)
	}

	allowed := false
	for _, geography := range strings.Split(geographies, ",") {
		normalized := strings.ToLower(strings.TrimSpace(geography))
		if normalized == "" || !isValidADXGeography(normalized) {
			return false, fmt.Errorf("adx-geographies has invalid entry %q", geography)
		}
		if normalized == currentGeography {
			allowed = true
		}
	}
	return allowed, nil
}

func isValidADXGeography(geography string) bool {
	for _, r := range geography {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			continue
		}
		return false
	}
	return true
}

func (o *RawAddDatasourceOptions) rawADXOptions() *RawReconcileADXDatasourceOptions {
	return &RawReconcileADXDatasourceOptions{
		BaseOptions:        o.BaseOptions,
		Enabled:            o.ADXEnabled,
		DeleteWhenDisabled: o.ADXDeleteWhenDisabled,
		ClusterURL:         o.ADXClusterURL,
		DefaultDatabase:    o.ADXDefaultDatabase,
		DatasourceName:     o.ADXDatasourceName,
		DataConsistency:    o.ADXDataConsistency,
	}
}

// Complete performs final initialization to create fully usable add datasource options.
func (o *ValidatedAddDatasourceOptions) Complete(ctx context.Context) (*CompletedAddDatasourceOptions, error) {
	var monitorWorkspaceClient *azure.MonitorWorkspaceClient
	var managedGrafanaClient *azure.ManagedGrafanaClient
	var grafanaClient adxGrafanaClient

	if o.AzureMonitorEnabled || o.ADXEnabled || o.ADXDeleteWhenDisabled {
		cred, err := cmdutils.GetAzureTokenCredentials()
		if err != nil {
			return nil, fmt.Errorf("failed to obtain Azure credentials: %w", err)
		}

		managedGrafanaClient, err = azure.NewManagedGrafanaClient(o.SubscriptionID, cred)
		if err != nil {
			return nil, fmt.Errorf("failed to create managed Grafana client: %w", err)
		}

		if o.AzureMonitorEnabled {
			monitorWorkspaceClient, err = azure.NewMonitorWorkspaceClient(o.SubscriptionID, cred)
			if err != nil {
				return nil, fmt.Errorf("failed to create monitor workspace client: %w", err)
			}
		}

		if o.ADXEnabled || o.ADXDeleteWhenDisabled {
			grafanaClient, err = grafana.NewClient(ctx, cred, managedGrafanaClient, o.SubscriptionID, o.ResourceGroup, o.GrafanaName)
			if err != nil {
				return nil, fmt.Errorf("failed to create Grafana client: %w", err)
			}
		}
	}

	return &CompletedAddDatasourceOptions{
		validatedAddDatasourceOptions: o.validatedAddDatasourceOptions,
		MonitorWorkspaceClient:        monitorWorkspaceClient,
		ManagedGrafanaClient:          managedGrafanaClient,
		GrafanaClient:                 grafanaClient,
	}, nil
}
