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

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Azure/ARO-Tools/tools/grafanactl/cmd/base"
	"github.com/Azure/ARO-Tools/tools/grafanactl/internal/azure"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/dashboard/armdashboard/v2"
)

func TestNormalizeResourceIDAndIntegrationFabricName(t *testing.T) {
	resourceID := " /SUBSCRIPTIONS/ABC/RESOURCEGROUPS/RG/PROVIDERS/MICROSOFT.KUSTO/CLUSTERS/LOGS/ "
	normalized := "/subscriptions/abc/resourcegroups/rg/providers/microsoft.kusto/clusters/logs"

	assert.Equal(t, normalized, normalizeResourceID(resourceID))
	assert.Equal(t, integrationFabricName(normalized), integrationFabricName(resourceID))
	assert.Equal(t, 20, len(integrationFabricName(resourceID)))
	assert.Regexp(t, `^adx-[0-9a-f]{16}$`, integrationFabricName(resourceID))
}

func TestParseGeographyAllowlist(t *testing.T) {
	assert.Equal(t, map[string]struct{}{"eus": {}, "wus": {}}, parseGeographyAllowlist(" EUS, wus, EUS "))
}

func TestSelectKustoClustersByGeography(t *testing.T) {
	succeededCluster := func(name, geography string) azure.KustoCluster {
		return azure.KustoCluster{
			ResourceID:        "/subscriptions/1/resourceGroups/rg/providers/Microsoft.Kusto/clusters/" + name,
			Geography:         geography,
			ProvisioningState: "Succeeded",
		}
	}

	t.Run("successful complete set", func(t *testing.T) {
		clusters := []azure.KustoCluster{
			succeededCluster("eus", "EUS"),
			succeededCluster("wus", "wus"),
			succeededCluster("extra", "cus"),
		}
		selected, err := selectKustoClustersByGeography(clusters, " wus, eus ")
		require.NoError(t, err)
		require.Len(t, selected, 2)
		assert.Equal(t, "eus", selected[0].Geography)
		assert.Equal(t, "wus", selected[1].Geography)
	})

	t.Run("missing requested geography", func(t *testing.T) {
		_, err := selectKustoClustersByGeography([]azure.KustoCluster{
			succeededCluster("wus", "wus"),
		}, "eus,wus")
		assert.ErrorContains(t, err, `requested geography "eus" has no matching managed Kusto cluster`)
	})

	t.Run("duplicate requested geography", func(t *testing.T) {
		_, err := selectKustoClustersByGeography([]azure.KustoCluster{
			succeededCluster("eus-1", "eus"),
			succeededCluster("eus-2", "EUS"),
		}, "eus")
		assert.ErrorContains(t, err, `requested geography "eus" has 2 matching managed Kusto clusters`)
	})

	t.Run("transitional requested cluster", func(t *testing.T) {
		cluster := succeededCluster("eus", "eus")
		cluster.ProvisioningState = "Updating"
		_, err := selectKustoClustersByGeography([]azure.KustoCluster{cluster}, "eus")
		assert.ErrorContains(t, err, `has provisioning state "Updating", not Succeeded`)
	})

	t.Run("missing geography tag on any managed cluster", func(t *testing.T) {
		_, err := selectKustoClustersByGeography([]azure.KustoCluster{
			succeededCluster("eus", "eus"),
			succeededCluster("untagged", ""),
		}, "eus")
		assert.ErrorContains(t, err, "missing the aroHCPGeoShortId tag")
	})
}

func TestPlanADXReconciliation(t *testing.T) {
	desired := []desiredADXIntegration{
		{Name: "adx-a", DataSourceResourceID: "/clusters/a", Scenario: "scenario"},
		{Name: "adx-b", DataSourceResourceID: "/clusters/b", Scenario: "scenario"},
		{Name: "adx-c", DataSourceResourceID: "/clusters/c", Scenario: "new"},
		{Name: "adx-d", DataSourceResourceID: "/clusters/d", TargetResourceID: "/targets/new"},
		{Name: "adx-f", DataSourceResourceID: "/clusters/new"},
	}
	existing := []armdashboard.IntegrationFabric{
		ownedFabric("adx-b", "/clusters/b", "scenario", ""),
		ownedFabric("adx-c", "/clusters/c", "OLD", ""),
		ownedFabric("adx-d", "/clusters/d", "", "/targets/old"),
		ownedFabric("adx-e", "/clusters/e", "", ""),
		ownedFabric("adx-f", "/clusters/old", "", ""),
		{
			Name: to.Ptr("unrelated"),
			Tags: map[string]*string{"owner": to.Ptr("someone-else")},
		},
	}

	operations, err := planADXReconciliation(desired, existing)
	require.NoError(t, err)
	assert.Equal(t, []adxReconcileOperationType{
		adxReconcileCreate,
		adxReconcileUpdate,
		adxReconcileRecreate,
		adxReconcileRecreate,
		adxReconcileDelete,
	}, operationTypes(operations))
	assert.Equal(t, []string{"adx-a", "adx-c", "adx-d", "adx-f", "adx-e"}, operationNames(operations))
}

func TestPlanADXReconciliationIgnoresScenarioCase(t *testing.T) {
	desired := []desiredADXIntegration{{
		Name:                 "adx-a",
		DataSourceResourceID: "/clusters/a",
		Scenario:             "scenario",
	}}
	existing := []armdashboard.IntegrationFabric{ownedFabric("adx-a", "/clusters/a", "SCENARIO", "")}

	operations, err := planADXReconciliation(desired, existing)
	require.NoError(t, err)
	assert.Empty(t, operations)
}

func TestPlanADXReconciliationIgnoresUnmanagedOptionalProperties(t *testing.T) {
	desired := []desiredADXIntegration{{
		Name:                 "adx-a",
		DataSourceResourceID: "/clusters/a",
	}}

	t.Run("scenario", func(t *testing.T) {
		existing := []armdashboard.IntegrationFabric{ownedFabric("adx-a", "/clusters/a", "rp-default", "")}

		operations, err := planADXReconciliation(desired, existing)
		require.NoError(t, err)
		assert.Empty(t, operations)
	})

	t.Run("target resource ID", func(t *testing.T) {
		existing := []armdashboard.IntegrationFabric{ownedFabric("adx-a", "/clusters/a", "", "/targets/rp-derived")}

		operations, err := planADXReconciliation(desired, existing)
		require.NoError(t, err)
		assert.Empty(t, operations)
	})
}

func TestPlanADXReconciliationPreservesUnownedNameConflict(t *testing.T) {
	desired := []desiredADXIntegration{{Name: "adx-a", DataSourceResourceID: "/clusters/a"}}
	existing := []armdashboard.IntegrationFabric{{
		Name: to.Ptr("adx-a"),
		Tags: map[string]*string{"owner": to.Ptr("someone-else")},
	}}

	_, err := planADXReconciliation(desired, existing)
	assert.ErrorContains(t, err, "is not owned by grafanactl")
}

func TestIntegrationFabricOwnershipFallback(t *testing.T) {
	resourceID := "/subscriptions/1/resourceGroups/rg/providers/Microsoft.Kusto/clusters/logs"
	fabric := armdashboard.IntegrationFabric{
		Name: to.Ptr(integrationFabricName(resourceID)),
		Properties: &armdashboard.IntegrationFabricProperties{
			DataSourceResourceID: to.Ptr(resourceID),
		},
	}
	assert.True(t, isOwnedIntegrationFabric(fabric))

	fabric.Tags = map[string]*string{"unrelated": to.Ptr("value")}
	assert.True(t, isOwnedIntegrationFabric(fabric))

	fabric.Tags = map[string]*string{
		adxPurposeTagKey:   to.Ptr(adxPurposeTagValue),
		adxManagedByTagKey: to.Ptr(adxManagedByTagValue),
	}
	assert.True(t, isOwnedIntegrationFabric(fabric))

	fabric.Tags[adxManagedByTagKey] = to.Ptr("someone-else")
	assert.False(t, isOwnedIntegrationFabric(fabric))

	delete(fabric.Tags, adxManagedByTagKey)
	assert.False(t, isOwnedIntegrationFabric(fabric))

	fabric.Tags = nil
	fabric.Name = to.Ptr(strings.ToUpper(integrationFabricName(resourceID)))
	assert.False(t, isOwnedIntegrationFabric(fabric))
}

func TestADXReconciliationDisabledMakesNoCalls(t *testing.T) {
	discovery := &fakeKustoDiscoveryClient{}
	fabrics := &fakeIntegrationFabricsClient{}
	options := &CompletedReconcileOptions{
		validatedReconcileOptions: &validatedReconcileOptions{
			RawReconcileOptions: &RawReconcileOptions{
				ADXIntegrationsEnabled: false,
			},
		},
		KustoDiscoveryClient:     discovery,
		IntegrationFabricsClient: fabrics,
	}

	require.NoError(t, options.reconcileADXIntegrations(t.Context(), "eastus", logr.Discard()))
	assert.Equal(t, 0, discovery.calls)
	assert.Equal(t, 0, fabrics.listCalls)
}

func TestADXReconciliationNoSelectedClustersMakesNoFabricCalls(t *testing.T) {
	discovery := &fakeKustoDiscoveryClient{
		clusters: []azure.KustoCluster{{
			ResourceID:        "/subscriptions/subscription-id/resourceGroups/rg/providers/Microsoft.Kusto/clusters/logs",
			Geography:         "wus",
			ProvisioningState: "Succeeded",
		}},
	}
	fabrics := &fakeIntegrationFabricsClient{}
	options := &CompletedReconcileOptions{
		validatedReconcileOptions: &validatedReconcileOptions{
			RawReconcileOptions: &RawReconcileOptions{
				BaseOptions:              &base.BaseOptions{SubscriptionID: "subscription-id"},
				ADXIntegrationsEnabled:   true,
				ADXEnvironment:           "int",
				ADXGeographies:           "eus",
				ADXScenario:              "",
				ADXTargetResourceID:      "",
				CrossTenantSecurityGroup: "",
			},
		},
		KustoDiscoveryClient:     discovery,
		IntegrationFabricsClient: fabrics,
	}

	err := options.reconcileADXIntegrations(t.Context(), "eastus", logr.Discard())
	assert.ErrorContains(t, err, `requested geography "eus" has no matching managed Kusto cluster`)
	assert.Equal(t, "subscription-id", discovery.subscriptionID)
	assert.Equal(t, "int", discovery.environment)
	assert.Equal(t, 0, fabrics.totalCalls())
}

func TestADXReconciliationDryRunPlansWithoutMutations(t *testing.T) {
	clusterResourceID := "/subscriptions/subscription-id/resourceGroups/rg/providers/Microsoft.Kusto/clusters/logs"
	discovery := &fakeKustoDiscoveryClient{
		clusters: []azure.KustoCluster{{
			ResourceID:        clusterResourceID,
			Geography:         "eus",
			ProvisioningState: "Succeeded",
		}},
	}
	fabrics := &fakeIntegrationFabricsClient{
		listErr: &azcore.ResponseError{StatusCode: 404},
	}
	options := completedADXOptions(discovery, fabrics, true)

	require.NoError(t, options.reconcileADXIntegrations(t.Context(), "eastus", logr.Discard()))
	assert.Equal(t, 1, fabrics.listCalls)
	assert.Equal(t, 0, fabrics.createCalls)
	assert.Equal(t, 0, fabrics.updateCalls)
	assert.Equal(t, 0, fabrics.deleteCalls)
}

func TestADXReconciliationPropagatesGrafanaNotFoundOutsideDryRun(t *testing.T) {
	clusterResourceID := "/subscriptions/subscription-id/resourceGroups/rg/providers/Microsoft.Kusto/clusters/logs"
	discovery := &fakeKustoDiscoveryClient{
		clusters: []azure.KustoCluster{{
			ResourceID:        clusterResourceID,
			Geography:         "eus",
			ProvisioningState: "Succeeded",
		}},
	}
	fabrics := &fakeIntegrationFabricsClient{
		listErr: &azcore.ResponseError{StatusCode: 404},
	}
	options := completedADXOptions(discovery, fabrics, false)

	err := options.reconcileADXIntegrations(t.Context(), "eastus", logr.Discard())
	assert.ErrorContains(t, err, "failed to list Grafana integration fabrics")
	assert.Equal(t, 1, fabrics.totalCalls())
}

func TestApplyADXReconciliationOperations(t *testing.T) {
	desired := &desiredADXIntegration{
		Name:                 "adx-create",
		DataSourceResourceID: "/clusters/logs",
		Scenario:             "scenario",
	}
	operations := []adxReconcileOperation{
		{Type: adxReconcileCreate, Name: "adx-create", Desired: desired},
		{Type: adxReconcileUpdate, Name: "adx-update", Desired: desired},
		{Type: adxReconcileRecreate, Name: "adx-recreate", Desired: desired},
		{Type: adxReconcileDelete, Name: "adx-delete"},
	}

	t.Run("apply", func(t *testing.T) {
		fabrics := &fakeIntegrationFabricsClient{}
		options := completedADXOptions(&fakeKustoDiscoveryClient{}, fabrics, false)
		require.NoError(t, options.applyADXReconciliationOperations(t.Context(), "eastus", operations, logr.Discard()))
		assert.Equal(t, 2, fabrics.createCalls)
		assert.Equal(t, 1, fabrics.updateCalls)
		assert.Equal(t, 2, fabrics.deleteCalls)
	})

	t.Run("dry run", func(t *testing.T) {
		fabrics := &fakeIntegrationFabricsClient{}
		options := completedADXOptions(&fakeKustoDiscoveryClient{}, fabrics, true)
		require.NoError(t, options.applyADXReconciliationOperations(t.Context(), "eastus", operations, logr.Discard()))
		assert.Equal(t, 0, fabrics.totalCalls())
	})
}

func TestIntegrationFabricResourceOmitsOptionalProperties(t *testing.T) {
	fabric := integrationFabricResource("eastus", desiredADXIntegration{
		DataSourceResourceID: "/clusters/logs",
	})

	require.NotNil(t, fabric.Properties)
	assert.Equal(t, "/clusters/logs", *fabric.Properties.DataSourceResourceID)
	assert.Nil(t, fabric.Properties.Scenarios)
	assert.Nil(t, fabric.Properties.TargetResourceID)
	assert.Equal(t, adxPurposeTagValue, *fabric.Tags[adxPurposeTagKey])
	assert.Equal(t, adxManagedByTagValue, *fabric.Tags[adxManagedByTagKey])
}

func ownedFabric(name, dataSourceResourceID, scenario, targetResourceID string) armdashboard.IntegrationFabric {
	properties := &armdashboard.IntegrationFabricProperties{
		DataSourceResourceID: to.Ptr(dataSourceResourceID),
	}
	if scenario != "" {
		properties.Scenarios = []*string{to.Ptr(scenario)}
	}
	if targetResourceID != "" {
		properties.TargetResourceID = to.Ptr(targetResourceID)
	}
	return armdashboard.IntegrationFabric{
		Name: to.Ptr(name),
		Tags: map[string]*string{
			adxPurposeTagKey:   to.Ptr(adxPurposeTagValue),
			adxManagedByTagKey: to.Ptr(adxManagedByTagValue),
		},
		Properties: properties,
	}
}

func operationTypes(operations []adxReconcileOperation) []adxReconcileOperationType {
	result := make([]adxReconcileOperationType, 0, len(operations))
	for _, operation := range operations {
		result = append(result, operation.Type)
	}
	return result
}

func operationNames(operations []adxReconcileOperation) []string {
	result := make([]string, 0, len(operations))
	for _, operation := range operations {
		result = append(result, operation.Name)
	}
	return result
}

func completedADXOptions(discovery *fakeKustoDiscoveryClient, fabrics *fakeIntegrationFabricsClient, dryRun bool) *CompletedReconcileOptions {
	return &CompletedReconcileOptions{
		validatedReconcileOptions: &validatedReconcileOptions{
			RawReconcileOptions: &RawReconcileOptions{
				BaseOptions: &base.BaseOptions{
					SubscriptionID: "subscription-id",
					ResourceGroup:  "resource-group",
					GrafanaName:    "grafana",
					DryRun:         dryRun,
				},
				ADXIntegrationsEnabled: true,
				ADXEnvironment:         "int",
				ADXGeographies:         "eus",
			},
		},
		KustoDiscoveryClient:     discovery,
		IntegrationFabricsClient: fabrics,
	}
}

type fakeKustoDiscoveryClient struct {
	calls          int
	subscriptionID string
	environment    string
	clusters       []azure.KustoCluster
}

func (f *fakeKustoDiscoveryClient) DiscoverKustoClusters(_ context.Context, subscriptionID, environment string) ([]azure.KustoCluster, error) {
	f.calls++
	f.subscriptionID = subscriptionID
	f.environment = environment
	return f.clusters, nil
}

type fakeIntegrationFabricsClient struct {
	listCalls   int
	createCalls int
	updateCalls int
	deleteCalls int
	existing    []armdashboard.IntegrationFabric
	listErr     error
}

func (f *fakeIntegrationFabricsClient) List(context.Context, string, string) ([]armdashboard.IntegrationFabric, error) {
	f.listCalls++
	return f.existing, f.listErr
}

func (f *fakeIntegrationFabricsClient) Create(context.Context, string, string, string, armdashboard.IntegrationFabric) (*armdashboard.IntegrationFabric, error) {
	f.createCalls++
	return nil, nil
}

func (f *fakeIntegrationFabricsClient) Update(context.Context, string, string, string, armdashboard.IntegrationFabricUpdateParameters) (*armdashboard.IntegrationFabric, error) {
	f.updateCalls++
	return nil, nil
}

func (f *fakeIntegrationFabricsClient) Delete(context.Context, string, string, string) error {
	f.deleteCalls++
	return nil
}

func (f *fakeIntegrationFabricsClient) totalCalls() int {
	return f.listCalls + f.createCalls + f.updateCalls + f.deleteCalls
}
