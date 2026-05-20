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

	"github.com/go-logr/logr"
	"github.com/grafana-tools/sdk"
)

const adxDatasourceType = "grafana-azure-data-explorer-datasource"

func (o *CompletedReconcileADXDatasourceOptions) Run(ctx context.Context) error {
	logger := logr.FromContextOrDiscard(ctx).WithValues(
		"resource-group", o.ResourceGroup,
		"grafana-name", o.GrafanaName,
		"datasource-name", o.DatasourceName,
	)

	logger.Info("reconcile ADX datasource command executed", "dry-run", o.DryRun)

	if !o.Enabled {
		return o.deleteDatasourceWhenDisabled(ctx, logger)
	}

	dataSourceTypes, err := o.GrafanaClient.ListDataSourceTypes(ctx)
	if err != nil {
		return fmt.Errorf("failed to list Grafana datasource plugins: %w", err)
	}
	if _, ok := dataSourceTypes[adxDatasourceType]; !ok {
		return fmt.Errorf("grafana datasource plugin %q is not available", adxDatasourceType)
	}

	existing, err := o.findExistingDatasource(ctx)
	if err != nil {
		return err
	}

	desired := o.desiredDatasource()
	if existing == nil {
		if o.DryRun {
			logger.Info("Dry run - would create ADX datasource")
			return nil
		}

		logger.Info("Creating ADX datasource")
		if err := o.GrafanaClient.CreateDataSource(ctx, desired); err != nil {
			// Handle race condition: if another process created the
			// datasource between our list and create calls, re-fetch
			// and fall through to the update path.
			existing, findErr := o.findExistingDatasource(ctx)
			if findErr != nil || existing == nil {
				return fmt.Errorf("failed to create ADX datasource %q: %w", o.DatasourceName, err)
			}
			logger.Info("Datasource was created concurrently, falling back to update",
				"datasource-id", existing.ID, "datasource-uid", existing.UID)
			desired.ID = existing.ID
			desired.UID = existing.UID
			desired.OrgID = existing.OrgID
			desired.IsDefault = existing.IsDefault
			if updateErr := o.GrafanaClient.UpdateDataSource(ctx, desired); updateErr != nil {
				return fmt.Errorf("failed to update ADX datasource %q after create conflict: %w", o.DatasourceName, updateErr)
			}
		}
		return nil
	}

	if existing.Type != adxDatasourceType {
		return fmt.Errorf("datasource %q already exists with type %q, expected %q", o.DatasourceName, existing.Type, adxDatasourceType)
	}

	desired.ID = existing.ID
	desired.UID = existing.UID
	desired.OrgID = existing.OrgID
	desired.IsDefault = existing.IsDefault

	if o.DryRun {
		logger.Info("Dry run - would update ADX datasource", "datasource-id", existing.ID, "datasource-uid", existing.UID)
		return nil
	}

	logger.Info("Updating ADX datasource", "datasource-id", existing.ID, "datasource-uid", existing.UID)
	if err := o.GrafanaClient.UpdateDataSource(ctx, desired); err != nil {
		return fmt.Errorf("failed to update ADX datasource %q: %w", o.DatasourceName, err)
	}

	return nil
}

func (o *CompletedReconcileADXDatasourceOptions) deleteDatasourceWhenDisabled(ctx context.Context, logger logr.Logger) error {
	if !o.DeleteWhenDisabled {
		logger.Info("ADX datasource desired state disabled and deletion disabled")
		return nil
	}

	existing, err := o.findExistingDatasource(ctx)
	if err != nil {
		return err
	}
	if existing == nil {
		logger.Info("ADX datasource desired state disabled and datasource is already absent")
		return nil
	}
	if existing.Type != adxDatasourceType {
		return fmt.Errorf("datasource %q already exists with type %q, expected %q", o.DatasourceName, existing.Type, adxDatasourceType)
	}
	if o.DryRun {
		logger.Info("Dry run - would delete ADX datasource", "datasource-id", existing.ID, "datasource-uid", existing.UID)
		return nil
	}

	logger.Info("Deleting ADX datasource", "datasource-id", existing.ID, "datasource-uid", existing.UID)
	if err := o.GrafanaClient.DeleteDataSource(ctx, o.DatasourceName); err != nil {
		return fmt.Errorf("failed to delete ADX datasource %q: %w", o.DatasourceName, err)
	}

	return nil
}

func (o *CompletedReconcileADXDatasourceOptions) findExistingDatasource(ctx context.Context) (*sdk.Datasource, error) {
	dataSources, err := o.GrafanaClient.ListDataSources(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list Grafana datasources: %w", err)
	}

	var existing *sdk.Datasource
	for i := range dataSources {
		if dataSources[i].Name != o.DatasourceName {
			continue
		}
		if existing != nil {
			return nil, fmt.Errorf("found multiple datasources named %q", o.DatasourceName)
		}
		existing = &dataSources[i]
	}

	return existing, nil
}

func (o *CompletedReconcileADXDatasourceOptions) desiredDatasource() sdk.Datasource {
	return sdk.Datasource{
		Name:   o.DatasourceName,
		Type:   adxDatasourceType,
		Access: "proxy",
		JSONData: map[string]interface{}{
			"clusterUrl":      o.ClusterURL,
			"defaultDatabase": o.DefaultDatabase,
			"dataConsistency": o.DataConsistency,
			"azureCredentials": map[string]interface{}{
				"authType": "msi",
			},
		},
	}
}
