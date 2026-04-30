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
	"strings"
	"testing"

	"github.com/grafana-tools/sdk"

	"github.com/Azure/ARO-Tools/tools/grafanactl/cmd/base"
)

type fakeADXGrafanaClient struct {
	dataSources     []sdk.Datasource
	dataSourceTypes map[string]sdk.DatasourceType
	createCalls     []sdk.Datasource
	updateCalls     []sdk.Datasource
	deleteCalls     []string
	listTypesErr    error
	listErr         error
	createErr       error
	updateErr       error
	deleteErr       error
}

func (f *fakeADXGrafanaClient) ListDataSources(ctx context.Context) ([]sdk.Datasource, error) {
	return f.dataSources, f.listErr
}

func (f *fakeADXGrafanaClient) ListDataSourceTypes(ctx context.Context) (map[string]sdk.DatasourceType, error) {
	return f.dataSourceTypes, f.listTypesErr
}

func (f *fakeADXGrafanaClient) CreateDataSource(ctx context.Context, dataSource sdk.Datasource) error {
	f.createCalls = append(f.createCalls, dataSource)
	if f.createErr != nil {
		return f.createErr
	}
	if dataSource.UID == "" {
		dataSource.UID = "created-uid"
	}
	f.dataSources = append(f.dataSources, dataSource)
	return nil
}

func (f *fakeADXGrafanaClient) UpdateDataSource(ctx context.Context, dataSource sdk.Datasource) error {
	f.updateCalls = append(f.updateCalls, dataSource)
	return f.updateErr
}

func (f *fakeADXGrafanaClient) DeleteDataSource(ctx context.Context, dataSourceName string) error {
	f.deleteCalls = append(f.deleteCalls, dataSourceName)
	return f.deleteErr
}

func validRawReconcileADXDatasourceOptions() *RawReconcileADXDatasourceOptions {
	baseOptions := base.DefaultBaseOptions()
	baseOptions.SubscriptionID = "subscription-id"
	baseOptions.ResourceGroup = "resource-group"
	baseOptions.GrafanaName = "grafana-name"

	return &RawReconcileADXDatasourceOptions{
		BaseOptions:     baseOptions,
		Enabled:         true,
		ClusterURL:      "https://example.kusto.windows.net",
		DefaultDatabase: "ServiceLogs",
		DatasourceName:  "kusto-int-uksouth",
	}
}

func completedReconcileADXDatasourceOptionsForTest(client adxGrafanaClient) *CompletedReconcileADXDatasourceOptions {
	raw := validRawReconcileADXDatasourceOptions()
	raw.DataConsistency = defaultADXDataConsistency
	return &CompletedReconcileADXDatasourceOptions{
		validatedReconcileADXDatasourceOptions: &validatedReconcileADXDatasourceOptions{
			RawReconcileADXDatasourceOptions: raw,
		},
		GrafanaClient: client,
	}
}

func TestReconcileADXDatasourceValidateDefaultsDataConsistency(t *testing.T) {
	validated, err := validRawReconcileADXDatasourceOptions().Validate(context.Background())
	if err != nil {
		t.Fatalf("Validate returned error: %v", err)
	}
	if validated.DataConsistency != defaultADXDataConsistency {
		t.Fatalf("expected default data consistency %q, got %q", defaultADXDataConsistency, validated.DataConsistency)
	}
}

func TestReconcileADXDatasourceValidateRejectsInvalidClusterURL(t *testing.T) {
	opts := validRawReconcileADXDatasourceOptions()
	opts.ClusterURL = "http://example.kusto.windows.net"

	_, err := opts.Validate(context.Background())
	if err == nil {
		t.Fatal("expected validation error")
	}
	if !strings.Contains(err.Error(), "absolute https URL") {
		t.Fatalf("expected https URL error, got %v", err)
	}
}

func TestReconcileADXDatasourceValidateAllowsDisabledWithoutClusterDetails(t *testing.T) {
	opts := validRawReconcileADXDatasourceOptions()
	opts.Enabled = false
	opts.ClusterURL = ""
	opts.DefaultDatabase = ""
	opts.DatasourceName = ""

	if _, err := opts.Validate(context.Background()); err != nil {
		t.Fatalf("Validate returned error: %v", err)
	}
}

func TestReconcileADXDatasourceValidateDisabledDeleteRequiresDatasourceName(t *testing.T) {
	opts := validRawReconcileADXDatasourceOptions()
	opts.Enabled = false
	opts.DeleteWhenDisabled = true
	opts.DatasourceName = ""

	_, err := opts.Validate(context.Background())
	if err == nil {
		t.Fatal("expected validation error")
	}
	if !strings.Contains(err.Error(), "datasource name is required") {
		t.Fatalf("expected datasource name error, got %v", err)
	}
}

func TestReconcileADXDatasourceRunCreatesDatasource(t *testing.T) {
	client := &fakeADXGrafanaClient{
		dataSourceTypes: map[string]sdk.DatasourceType{
			adxDatasourceType: {},
		},
	}
	opts := completedReconcileADXDatasourceOptionsForTest(client)

	if err := opts.Run(context.Background()); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if len(client.createCalls) != 1 {
		t.Fatalf("expected one create call, got %d", len(client.createCalls))
	}
	if len(client.updateCalls) != 0 {
		t.Fatalf("expected no update calls, got %d", len(client.updateCalls))
	}
	if len(client.deleteCalls) != 0 {
		t.Fatalf("expected no delete calls, got %d", len(client.deleteCalls))
	}
	created := client.createCalls[0]
	if created.Name != opts.DatasourceName {
		t.Fatalf("expected datasource name %q, got %q", opts.DatasourceName, created.Name)
	}
	if created.Type != adxDatasourceType {
		t.Fatalf("expected datasource type %q, got %q", adxDatasourceType, created.Type)
	}
	jsonData, ok := created.JSONData.(map[string]interface{})
	if !ok {
		t.Fatalf("expected JSONData map, got %T", created.JSONData)
	}
	if jsonData["clusterUrl"] != opts.ClusterURL {
		t.Fatalf("expected clusterUrl %q, got %v", opts.ClusterURL, jsonData["clusterUrl"])
	}
	azureCredentials, ok := jsonData["azureCredentials"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected azureCredentials map, got %T", jsonData["azureCredentials"])
	}
	if azureCredentials["authType"] != "msi" {
		t.Fatalf("expected MSI auth type, got %v", azureCredentials["authType"])
	}
}

func TestReconcileADXDatasourceRunUpdatesExistingDatasource(t *testing.T) {
	client := &fakeADXGrafanaClient{
		dataSourceTypes: map[string]sdk.DatasourceType{
			adxDatasourceType: {},
		},
		dataSources: []sdk.Datasource{
			{
				ID:        42,
				OrgID:     7,
				UID:       "existing-uid",
				Name:      "kusto-int-uksouth",
				Type:      adxDatasourceType,
				Access:    "direct",
				IsDefault: true,
			},
		},
	}
	opts := completedReconcileADXDatasourceOptionsForTest(client)

	if err := opts.Run(context.Background()); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if len(client.createCalls) != 0 {
		t.Fatalf("expected no create calls, got %d", len(client.createCalls))
	}
	if len(client.updateCalls) != 1 {
		t.Fatalf("expected one update call, got %d", len(client.updateCalls))
	}

	updated := client.updateCalls[0]
	if updated.ID != 42 || updated.OrgID != 7 || updated.UID != "existing-uid" || !updated.IsDefault {
		t.Fatalf("expected existing datasource identity/default to be preserved, got %#v", updated)
	}
}

func TestReconcileADXDatasourceRunDeletesWhenDisabled(t *testing.T) {
	client := &fakeADXGrafanaClient{
		dataSources: []sdk.Datasource{
			{
				ID:   42,
				UID:  "existing-uid",
				Name: "kusto-int-uksouth",
				Type: adxDatasourceType,
			},
		},
	}
	opts := completedReconcileADXDatasourceOptionsForTest(client)
	opts.Enabled = false
	opts.DeleteWhenDisabled = true

	if err := opts.Run(context.Background()); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if len(client.createCalls) != 0 {
		t.Fatalf("expected no create calls, got %d", len(client.createCalls))
	}
	if len(client.updateCalls) != 0 {
		t.Fatalf("expected no update calls, got %d", len(client.updateCalls))
	}
	if len(client.deleteCalls) != 1 || client.deleteCalls[0] != "kusto-int-uksouth" {
		t.Fatalf("expected datasource delete call, got %#v", client.deleteCalls)
	}
}

func TestReconcileADXDatasourceRunDisabledDeleteIgnoresAbsentDatasource(t *testing.T) {
	client := &fakeADXGrafanaClient{}
	opts := completedReconcileADXDatasourceOptionsForTest(client)
	opts.Enabled = false
	opts.DeleteWhenDisabled = true

	if err := opts.Run(context.Background()); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if len(client.deleteCalls) != 0 {
		t.Fatalf("expected no delete calls, got %d", len(client.deleteCalls))
	}
}

func TestReconcileADXDatasourceRunDisabledWithoutDeleteDoesNothing(t *testing.T) {
	client := &fakeADXGrafanaClient{}
	opts := completedReconcileADXDatasourceOptionsForTest(client)
	opts.Enabled = false
	opts.DeleteWhenDisabled = false

	if err := opts.Run(context.Background()); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if len(client.createCalls) != 0 || len(client.updateCalls) != 0 || len(client.deleteCalls) != 0 {
		t.Fatalf("expected no mutation calls, got create=%d update=%d delete=%d", len(client.createCalls), len(client.updateCalls), len(client.deleteCalls))
	}
}

func TestReconcileADXDatasourceRunRejectsExistingDatasourceWithWrongType(t *testing.T) {
	client := &fakeADXGrafanaClient{
		dataSourceTypes: map[string]sdk.DatasourceType{
			adxDatasourceType: {},
		},
		dataSources: []sdk.Datasource{
			{
				Name: "kusto-int-uksouth",
				Type: "prometheus",
			},
		},
	}
	opts := completedReconcileADXDatasourceOptionsForTest(client)

	err := opts.Run(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "already exists with type") {
		t.Fatalf("expected wrong type error, got %v", err)
	}
}

func TestReconcileADXDatasourceRunRejectsMissingPlugin(t *testing.T) {
	client := &fakeADXGrafanaClient{
		dataSourceTypes: map[string]sdk.DatasourceType{},
	}
	opts := completedReconcileADXDatasourceOptionsForTest(client)

	err := opts.Run(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "is not available") {
		t.Fatalf("expected missing plugin error, got %v", err)
	}
}

func TestReconcileADXDatasourceRunPropagatesMutationErrors(t *testing.T) {
	client := &fakeADXGrafanaClient{
		dataSourceTypes: map[string]sdk.DatasourceType{
			adxDatasourceType: {},
		},
		createErr: errors.New("status 403"),
	}
	opts := completedReconcileADXDatasourceOptionsForTest(client)

	err := opts.Run(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "failed to create ADX datasource") {
		t.Fatalf("expected create error, got %v", err)
	}
}

func TestAddDatasourceValidateDisablesADXWhenCurrentGeographyNotAllowed(t *testing.T) {
	opts := DefaultAddDatasourceOptions()
	opts.SubscriptionID = "subscription-id"
	opts.ResourceGroup = "resource-group"
	opts.GrafanaName = "grafana-name"
	opts.AzureMonitorEnabled = false
	opts.ADXEnabled = true
	opts.ADXDeleteWhenDisabled = true
	opts.ADXDatasourceName = "kusto-int-uksouth"
	opts.ADXGeographies = "eus2, wus3"
	opts.ADXCurrentGeography = "UKS"

	validated, err := opts.Validate(context.Background())
	if err != nil {
		t.Fatalf("Validate returned error: %v", err)
	}
	if validated.ADXEnabled {
		t.Fatal("expected ADX to be disabled for disallowed geography")
	}
	if !validated.ADXDeleteWhenDisabled {
		t.Fatal("expected deleteWhenDisabled to remain enabled")
	}
}

func TestAddDatasourceValidateAllowsADXWhenCurrentGeographyAllowed(t *testing.T) {
	opts := DefaultAddDatasourceOptions()
	opts.SubscriptionID = "subscription-id"
	opts.ResourceGroup = "resource-group"
	opts.GrafanaName = "grafana-name"
	opts.AzureMonitorEnabled = false
	opts.ADXEnabled = true
	opts.ADXClusterURL = "https://example.kusto.windows.net"
	opts.ADXDefaultDatabase = "ServiceLogs"
	opts.ADXDatasourceName = "kusto-int-uksouth"
	opts.ADXGeographies = " eus2, UKS "
	opts.ADXCurrentGeography = "uks"

	validated, err := opts.Validate(context.Background())
	if err != nil {
		t.Fatalf("Validate returned error: %v", err)
	}
	if !validated.ADXEnabled {
		t.Fatal("expected ADX to remain enabled for allowed geography")
	}
}

func TestAddDatasourceValidateRejectsInvalidADXGeographies(t *testing.T) {
	opts := DefaultAddDatasourceOptions()
	opts.SubscriptionID = "subscription-id"
	opts.ResourceGroup = "resource-group"
	opts.GrafanaName = "grafana-name"
	opts.AzureMonitorEnabled = false
	opts.ADXEnabled = true
	opts.ADXGeographies = "uks,!"
	opts.ADXCurrentGeography = "uks"

	_, err := opts.Validate(context.Background())
	if err == nil {
		t.Fatal("expected validation error")
	}
	if !strings.Contains(err.Error(), "invalid entry") {
		t.Fatalf("expected invalid geography error, got %v", err)
	}
}

func TestAddDatasourceValidateIgnoresADXGeographiesWhenADXDisabled(t *testing.T) {
	opts := DefaultAddDatasourceOptions()
	opts.SubscriptionID = "subscription-id"
	opts.ResourceGroup = "resource-group"
	opts.GrafanaName = "grafana-name"
	opts.AzureMonitorEnabled = false
	opts.ADXEnabled = false
	opts.ADXGeographies = "uks,!"

	validated, err := opts.Validate(context.Background())
	if err != nil {
		t.Fatalf("Validate returned error: %v", err)
	}
	if validated.ADXEnabled {
		t.Fatal("expected ADX to remain disabled")
	}
}
