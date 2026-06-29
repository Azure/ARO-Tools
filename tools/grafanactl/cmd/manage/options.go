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

package manage

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/Azure/ARO-Tools/tools/cmdutils"
	"github.com/Azure/ARO-Tools/tools/grafanactl/cmd/base"
	"github.com/Azure/ARO-Tools/tools/grafanactl/internal/azure"
)

// RawReconcileOptions represents the initial, unvalidated configuration for reconcile operations.
type RawReconcileOptions struct {
	*base.BaseOptions
	Location                 string
	SKU                      string
	MajorVersion             string
	ZoneRedundancy           string
	CrossTenantSecurityGroup string
	WorkspacePrefixes        []string
}

type validatedReconcileOptions struct {
	*RawReconcileOptions
	*base.CompletedBaseOptions
}

// ValidatedReconcileOptions represents reconcile configuration that has passed validation.
type ValidatedReconcileOptions struct {
	*validatedReconcileOptions
}

// CompletedReconcileOptions represents the final, fully validated and initialized configuration
// for reconcile operations.
type CompletedReconcileOptions struct {
	*validatedReconcileOptions
	ManagedGrafanaClient        *azure.ManagedGrafanaClient
	ResourceGraphDiscoveryClient *azure.ResourceGraphDiscoveryClient
}

// DefaultReconcileOptions returns a new RawReconcileOptions with default values
func DefaultReconcileOptions() *RawReconcileOptions {
	return &RawReconcileOptions{
		BaseOptions:    base.DefaultBaseOptions(),
		SKU:            "Standard",
		ZoneRedundancy: "Disabled",
	}
}

// BindReconcileOptions binds command-line flags to the options
func BindReconcileOptions(opts *RawReconcileOptions, cmd *cobra.Command) error {
	if err := base.BindBaseOptions(opts.BaseOptions, cmd); err != nil {
		return err
	}

	flags := cmd.Flags()
	flags.StringVar(&opts.Location, "location", opts.Location, "Azure region for the Grafana instance")
	flags.StringVar(&opts.SKU, "sku", opts.SKU, "Grafana SKU name (e.g. Standard)")
	flags.StringVar(&opts.MajorVersion, "major-version", opts.MajorVersion, "Grafana major version (e.g. 11)")
	flags.StringVar(&opts.ZoneRedundancy, "zone-redundancy", opts.ZoneRedundancy, "Zone redundancy mode: Enabled or Disabled")
	flags.StringVar(&opts.CrossTenantSecurityGroup, "cross-tenant-security-group", opts.CrossTenantSecurityGroup, "Cross-tenant security group (format: GroupObjectId;TenantId)")
	flags.StringSliceVar(&opts.WorkspacePrefixes, "workspace-prefixes", opts.WorkspacePrefixes, "Name prefixes to filter Azure Monitor Workspaces by (comma-separated)")

	return nil
}

// Validate performs validation on the raw options
func (o *RawReconcileOptions) Validate(ctx context.Context) (*ValidatedReconcileOptions, error) {
	completedBase, err := base.ValidateBaseOptions(o.BaseOptions)
	if err != nil {
		return nil, err
	}

	if o.Location == "" {
		return nil, fmt.Errorf("--location is required")
	}

	if o.SKU == "" {
		return nil, fmt.Errorf("--sku is required")
	}

	if o.ZoneRedundancy != "Enabled" && o.ZoneRedundancy != "Disabled" {
		return nil, fmt.Errorf("--zone-redundancy must be 'Enabled' or 'Disabled', got: %s", o.ZoneRedundancy)
	}

	for _, prefix := range o.WorkspacePrefixes {
		if prefix == "" {
			return nil, fmt.Errorf("--workspace-prefixes must not contain empty strings")
		}
	}

	return &ValidatedReconcileOptions{
		validatedReconcileOptions: &validatedReconcileOptions{
			RawReconcileOptions:  o,
			CompletedBaseOptions: completedBase,
		},
	}, nil
}

// Complete performs final initialization to create fully usable reconcile options.
func (o *ValidatedReconcileOptions) Complete(ctx context.Context) (*CompletedReconcileOptions, error) {
	cred, err := cmdutils.GetAzureTokenCredentialsForCloud(o.CloudConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to obtain Azure credentials: %w", err)
	}

	clientOpts := o.ARMClientOptions()

	managedGrafanaClient, err := azure.NewManagedGrafanaClient(o.SubscriptionID, cred, clientOpts)
	if err != nil {
		return nil, fmt.Errorf("failed to create managed Grafana client: %w", err)
	}

	resourceGraphClient, err := azure.NewResourceGraphDiscoveryClient(cred, clientOpts)
	if err != nil {
		return nil, fmt.Errorf("failed to create Resource Graph discovery client: %w", err)
	}

	return &CompletedReconcileOptions{
		validatedReconcileOptions:    o.validatedReconcileOptions,
		ManagedGrafanaClient:         managedGrafanaClient,
		ResourceGraphDiscoveryClient: resourceGraphClient,
	}, nil
}
