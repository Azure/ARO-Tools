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

	"github.com/Azure/ARO-Tools/tools/grafanactl/cmd/base"
)

const defaultADXDataConsistency = "strongconsistency"

type adxGrafanaClient interface {
	ListDataSources(ctx context.Context) ([]sdk.Datasource, error)
	ListDataSourceTypes(ctx context.Context) (map[string]sdk.DatasourceType, error)
	CreateDataSource(ctx context.Context, dataSource sdk.Datasource) error
	UpdateDataSource(ctx context.Context, dataSource sdk.Datasource) error
	DeleteDataSource(ctx context.Context, dataSourceName string) error
}

// RawReconcileADXDatasourceOptions represents the initial, unvalidated configuration for ADX datasource reconcile operations.
type RawReconcileADXDatasourceOptions struct {
	*base.BaseOptions
	Enabled            bool
	DeleteWhenDisabled bool
	ClusterURL         string
	DefaultDatabase    string
	DatasourceName     string
	DataConsistency    string
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

	if !o.Enabled {
		if o.DeleteWhenDisabled && datasourceName == "" {
			return nil, fmt.Errorf("datasource name is required when ADX datasource deletion is enabled")
		}
		return &ValidatedReconcileADXDatasourceOptions{
			validatedReconcileADXDatasourceOptions: &validatedReconcileADXDatasourceOptions{
				RawReconcileADXDatasourceOptions: &RawReconcileADXDatasourceOptions{
					BaseOptions:        o.BaseOptions,
					Enabled:            false,
					DeleteWhenDisabled: o.DeleteWhenDisabled,
					DatasourceName:     datasourceName,
					DataConsistency:    dataConsistency,
				},
			},
		}, nil
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
				BaseOptions:        o.BaseOptions,
				Enabled:            true,
				DeleteWhenDisabled: o.DeleteWhenDisabled,
				ClusterURL:         clusterURL,
				DefaultDatabase:    defaultDatabase,
				DatasourceName:     datasourceName,
				DataConsistency:    dataConsistency,
			},
		},
	}, nil
}
