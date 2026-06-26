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
	"errors"
	"fmt"
	"strings"

	"github.com/go-logr/logr"
	"github.com/spf13/cobra"

	"k8s.io/utils/set"

	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/monitor/armmonitor"
)

const datasourceGroupID = "datasource"

func NewModifyCommand(group string) (*cobra.Command, error) {
	opts := DefaultAddDatasourceOptions()

	modifyCmd := &cobra.Command{
		Use:     "modify",
		Short:   "Modify Grafana resources",
		Long:    "Modify Grafana dashboards, data sources, or other resources.",
		GroupID: group,
	}

	modifyCmd.AddGroup(&cobra.Group{
		ID:    datasourceGroupID,
		Title: "Datasource Commands:",
	})

	datasourceCmd := &cobra.Command{
		Use:     "datasource",
		Short:   "Manage Grafana datasources",
		Long:    "Add, update, or manage Grafana datasources.",
		GroupID: datasourceGroupID,
	}

	addDatasourceCmd := &cobra.Command{
		Use:   "reconcile",
		Short: "Reconcile Azure Monitor Workspace datasources in Grafana",
		Long:  "Reconcile Azure Monitor Workspace datasources in the Azure Managed Grafana instance. This integrates the workspaces with Grafana and creates the necessary datasource configuration.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return opts.Run(cmd.Context())
		},
	}

	if err := BindAddDatasourceOptions(opts, addDatasourceCmd); err != nil {
		return nil, err
	}

	datasourceCmd.AddCommand(addDatasourceCmd)
	modifyCmd.AddCommand(datasourceCmd)

	return modifyCmd, nil
}

func (opts *RawAddDatasourceOptions) Run(ctx context.Context) error {
	validated, err := opts.Validate(ctx)
	if err != nil {
		return fmt.Errorf("validation failed: %w", err)
	}

	completed, err := validated.Complete(ctx)
	if err != nil {
		return fmt.Errorf("completion failed: %w", err)
	}

	return completed.Run(ctx)
}

func isTerminalFailureState(state armmonitor.ProvisioningState) bool {
	switch state {
	case armmonitor.ProvisioningStateFailed, armmonitor.ProvisioningStateCanceled:
		return true
	default:
		return false
	}
}

func getMatchingWorkspaceIDs(workspaces []armmonitor.AzureMonitorWorkspaceResource, logger logr.Logger) set.Set[string] {
	validWorkspaceIDs := set.New[string]()

	for _, workspace := range workspaces {
		if workspace.Properties == nil || workspace.Properties.ProvisioningState == nil || workspace.ID == nil {
			continue
		}
		state := *workspace.Properties.ProvisioningState
		if isTerminalFailureState(state) {
			logger.Info("Skipping workspace in terminal failure state", "workspace-id", *workspace.ID, "provisioning-state", state)
			continue
		}
		logger.Info("Found", "workspace-id", *workspace.ID, "provisioning-state", state)
		validWorkspaceIDs.Insert(strings.ToLower(*workspace.ID))
	}

	return validWorkspaceIDs
}

func getActiveWorkspaceNames(workspaces []armmonitor.AzureMonitorWorkspaceResource) set.Set[string] {
	names := set.New[string]()

	for _, workspace := range workspaces {
		if workspace.Name == nil || workspace.Properties == nil || workspace.Properties.ProvisioningState == nil {
			continue
		}
		if !isTerminalFailureState(*workspace.Properties.ProvisioningState) {
			names.Insert(strings.ToLower(*workspace.Name))
		}
	}

	return names
}

func getWorkspaceEndpoints(workspaces []armmonitor.AzureMonitorWorkspaceResource, logger logr.Logger) map[string]string {
	endpoints := make(map[string]string)

	for _, workspace := range workspaces {
		if workspace.Name == nil || workspace.Properties == nil ||
			workspace.Properties.ProvisioningState == nil ||
			workspace.Properties.Metrics == nil ||
			workspace.Properties.Metrics.PrometheusQueryEndpoint == nil {
			continue
		}
		if *workspace.Properties.ProvisioningState == armmonitor.ProvisioningStateSucceeded {
			name := strings.ToLower(*workspace.Name)
			endpoints[name] = *workspace.Properties.Metrics.PrometheusQueryEndpoint
			logger.Info("Found workspace endpoint", "workspace-name", *workspace.Name, "endpoint", endpoints[name])
		}
	}

	return endpoints
}

func (o *CompletedAddDatasourceOptions) reconcileDatasources(ctx context.Context, logger logr.Logger, activeWorkspaceNames set.Set[string], workspaceEndpoints map[string]string) error {
	datasources, err := o.GrafanaClient.ListDataSources(ctx)
	if err != nil {
		return fmt.Errorf("failed to list Grafana datasources: %w", err)
	}

	var reconcileErrors error
	for _, ds := range datasources {
		if ds.Type != "prometheus" {
			continue
		}

		workspaceName := strings.TrimPrefix(ds.Name, "Managed_Prometheus_")
		if workspaceName == ds.Name {
			continue
		}

		lowerName := strings.ToLower(workspaceName)

		if !activeWorkspaceNames.Has(lowerName) {
			if o.DryRun {
				logger.Info("Dry run - would delete orphaned datasource", "datasource-name", ds.Name)
				continue
			}

			logger.Info("Deleting orphaned datasource", "datasource-name", ds.Name)
			if err := o.GrafanaClient.DeleteDataSource(ctx, ds.Name); err != nil {
				reconcileErrors = errors.Join(reconcileErrors, fmt.Errorf("failed to delete datasource %q: %w", ds.Name, err))
			}
			continue
		}

		expectedEndpoint, ok := workspaceEndpoints[lowerName]
		if !ok {
			logger.Info("Workspace exists but has no Prometheus endpoint yet, skipping", "datasource-name", ds.Name)
			continue
		}

		if ds.URL == expectedEndpoint {
			logger.Info("Datasource URL is current", "datasource-name", ds.Name, "url", ds.URL)
			continue
		}

		if o.DryRun {
			logger.Info("Dry run - would update stale datasource URL",
				"datasource-name", ds.Name,
				"current-url", ds.URL,
				"expected-url", expectedEndpoint)
			continue
		}

		logger.Info("Updating stale datasource URL",
			"datasource-name", ds.Name,
			"current-url", ds.URL,
			"expected-url", expectedEndpoint)

		ds.URL = expectedEndpoint
		if err := o.GrafanaClient.UpdateDataSource(ctx, ds); err != nil {
			reconcileErrors = errors.Join(reconcileErrors, fmt.Errorf("failed to update datasource %q URL: %w", ds.Name, err))
		}
	}

	if reconcileErrors != nil {
		return fmt.Errorf("failed to reconcile datasources: %w", reconcileErrors)
	}

	return nil
}

func (o *CompletedAddDatasourceOptions) Run(ctx context.Context) error {
	logger := logr.FromContextOrDiscard(ctx).WithValues("resource-group", o.ResourceGroup, "grafana-name", o.GrafanaName)

	logger.Info("add datasource command executed")

	grafana, err := o.ManagedGrafanaClient.GetGrafanaInstance(ctx, o.ResourceGroup, o.GrafanaName)
	if err != nil {
		return fmt.Errorf("failed to get Grafana instance: %w", err)
	}

	monitorWorkspaces, err := o.MonitorWorkspaceClient.GetAllMonitorWorkspaces(ctx)
	if err != nil {
		return fmt.Errorf("failed to list Azure Monitor Workspaces: %w", err)
	}

	validWorkspaceIDs := getMatchingWorkspaceIDs(monitorWorkspaces, logger)

	integrationList := set.New[string]()
	for _, integration := range grafana.Properties.GrafanaIntegrations.AzureMonitorWorkspaceIntegrations {
		if integration.AzureMonitorWorkspaceResourceID == nil {
			return fmt.Errorf("got nil resource ID for integration, this looks like a bug")
		}
		integrationID := strings.ToLower(*integration.AzureMonitorWorkspaceResourceID)
		if validWorkspaceIDs.Has(integrationID) {
			integrationList.Insert(integrationID)
		} else {
			logger.Info("Removing", "workspace-id", integrationID)
		}
	}

	for _, workspaceID := range validWorkspaceIDs.UnsortedList() {
		if !integrationList.Has(workspaceID) {
			logger.Info("Adding", "workspace-id", workspaceID)
			integrationList.Insert(workspaceID)
		}
	}

	if o.DryRun {
		logger.Info("Dry run - would reconcile Azure Monitor Workspace integrations", "total-integrations", integrationList.Len())
	} else {
		logger.Info("Reconciling Azure Monitor Workspace integrations", "total-integrations", integrationList.Len())

		err = o.ManagedGrafanaClient.UpdateGrafanaIntegrations(ctx, o.ResourceGroup, o.GrafanaName, integrationList.UnsortedList())
		if err != nil {
			return fmt.Errorf("failed to update Grafana integrations: %w", err)
		}
	}

	activeWorkspaceNames := getActiveWorkspaceNames(monitorWorkspaces)
	workspaceEndpoints := getWorkspaceEndpoints(monitorWorkspaces, logger)

	logger.Info("Reconciling datasources")
	if err := o.reconcileDatasources(ctx, logger, activeWorkspaceNames, workspaceEndpoints); err != nil {
		return fmt.Errorf("failed to reconcile datasources: %w", err)
	}

	return nil
}
