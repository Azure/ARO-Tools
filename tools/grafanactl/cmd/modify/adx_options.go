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
	"net/url"
	"strings"

	"github.com/grafana-tools/sdk"
	"github.com/spf13/cobra"

	"github.com/Azure/ARO-Tools/tools/cmdutils"
	"github.com/Azure/ARO-Tools/tools/grafanactl/cmd/base"
	"github.com/Azure/ARO-Tools/tools/grafanactl/internal/azure"
	"github.com/Azure/ARO-Tools/tools/grafanactl/internal/grafana"
)

const defaultADXDataConsistency = "strongconsistency"

type adxGrafanaClient interface {
	ListDataSources(ctx context.Context) ([]sdk.Datasource, error)
	ListDataSourceTypes(ctx context.Context) (map[string]sdk.DatasourceType, error)
	CreateDataSource(ctx context.Context, dataSource sdk.Datasource) error
	UpdateDataSource(ctx context.Context, dataSource sdk.Datasource) error
}

// RawReconcileADXDatasourceOptions represents the initial, unvalidated configuration for ADX datasource reconcile operations.
type RawReconcileADXDatasourceOptions struct {
	*base.BaseOptions
	ClusterURL      string
	DefaultDatabase string
	DatasourceName  string
	DataConsistency string
}

type validatedReconcileADXDatasourceOptions struct {
	*RawReconcileADXDatasourceOptions
}

// ValidatedReconcileADXDatasourceOptions represents ADX datasource reconcile configuration that has passed validation.
type ValidatedReconcileADXDatasourceOptions struct {
	*validatedReconcileADXDatasourceOptions
}

// CompletedReconcileADXDatasourceOptions represents fully initialized ADX datasource reconcile configuration.
type CompletedReconcileADXDatasourceOptions struct {
	*validatedReconcileADXDatasourceOptions
	GrafanaClient adxGrafanaClient
}

// DefaultReconcileADXDatasourceOptions returns a new RawReconcileADXDatasourceOptions with default values.
func DefaultReconcileADXDatasourceOptions() *RawReconcileADXDatasourceOptions {
	return &RawReconcileADXDatasourceOptions{
		BaseOptions:     base.DefaultBaseOptions(),
		DataConsistency: defaultADXDataConsistency,
	}
}

// BindReconcileADXDatasourceOptions binds command-line flags to the options.
func BindReconcileADXDatasourceOptions(opts *RawReconcileADXDatasourceOptions, cmd *cobra.Command) error {
	if err := base.BindBaseOptions(opts.BaseOptions, cmd); err != nil {
		return err
	}

	flags := cmd.Flags()
	flags.StringVar(&opts.ClusterURL, "cluster-url", opts.ClusterURL, "Azure Data Explorer cluster URL")
	flags.StringVar(&opts.DefaultDatabase, "default-database", opts.DefaultDatabase, "Default Azure Data Explorer database")
	flags.StringVar(&opts.DatasourceName, "datasource-name", opts.DatasourceName, "Grafana datasource name")
	flags.StringVar(&opts.DataConsistency, "data-consistency", opts.DataConsistency, "Azure Data Explorer datasource data consistency")

	return nil
}

// Validate performs validation on the raw options.
func (o *RawReconcileADXDatasourceOptions) Validate(ctx context.Context) (*ValidatedReconcileADXDatasourceOptions, error) {
	if err := base.ValidateBaseOptions(o.BaseOptions); err != nil {
		return nil, err
	}

	clusterURL := strings.TrimSpace(o.ClusterURL)
	defaultDatabase := strings.TrimSpace(o.DefaultDatabase)
	datasourceName := strings.TrimSpace(o.DatasourceName)
	dataConsistency := strings.TrimSpace(o.DataConsistency)
	if dataConsistency == "" {
		dataConsistency = defaultADXDataConsistency
	}

	if clusterURL == "" {
		return nil, fmt.Errorf("cluster URL is required")
	}
	parsedClusterURL, err := url.Parse(clusterURL)
	if err != nil {
		return nil, fmt.Errorf("invalid cluster URL: %w", err)
	}
	if parsedClusterURL.Scheme != "https" || parsedClusterURL.Host == "" {
		return nil, fmt.Errorf("cluster URL must be an absolute https URL")
	}
	if defaultDatabase == "" {
		return nil, fmt.Errorf("default database is required")
	}
	if datasourceName == "" {
		return nil, fmt.Errorf("datasource name is required")
	}

	return &ValidatedReconcileADXDatasourceOptions{
		validatedReconcileADXDatasourceOptions: &validatedReconcileADXDatasourceOptions{
			RawReconcileADXDatasourceOptions: &RawReconcileADXDatasourceOptions{
				BaseOptions:     o.BaseOptions,
				ClusterURL:      clusterURL,
				DefaultDatabase: defaultDatabase,
				DatasourceName:  datasourceName,
				DataConsistency: dataConsistency,
			},
		},
	}, nil
}

// Complete performs final initialization to create fully usable ADX datasource reconcile options.
func (o *ValidatedReconcileADXDatasourceOptions) Complete(ctx context.Context) (*CompletedReconcileADXDatasourceOptions, error) {
	cred, err := cmdutils.GetAzureTokenCredentials()
	if err != nil {
		return nil, fmt.Errorf("failed to obtain Azure credentials: %w", err)
	}

	managedGrafanaClient, err := azure.NewManagedGrafanaClient(o.SubscriptionID, cred)
	if err != nil {
		return nil, fmt.Errorf("failed to create managed Grafana client: %w", err)
	}

	grafanaClient, err := grafana.NewClient(ctx, cred, managedGrafanaClient, o.SubscriptionID, o.ResourceGroup, o.GrafanaName)
	if err != nil {
		return nil, fmt.Errorf("failed to create Grafana client: %w", err)
	}

	return &CompletedReconcileADXDatasourceOptions{
		validatedReconcileADXDatasourceOptions: o.validatedReconcileADXDatasourceOptions,
		GrafanaClient:                          grafanaClient,
	}, nil
}
