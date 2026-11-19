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

package azure

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/fake"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/monitor/armmonitor"
	monitorfake "github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/monitor/armmonitor/fake"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resourcegraph/armresourcegraph"
)

func TestGetPrometheusQueryEndpoint(t *testing.T) {
	readError := errors.New("workspace read failed")
	for _, test := range []struct {
		name       string
		properties *armmonitor.AzureMonitorWorkspaceResourceProperties
		readError  error
		want       string
		wantError  string
	}{
		{
			name: "current endpoint",
			properties: &armmonitor.AzureMonitorWorkspaceResourceProperties{
				Metrics: &armmonitor.AzureMonitorWorkspaceMetrics{PrometheusQueryEndpoint: to.Ptr("https://current.example.com")},
			},
			want: "https://current.example.com",
		},
		{name: "missing properties", wantError: "no Prometheus query endpoint"},
		{name: "missing metrics", properties: &armmonitor.AzureMonitorWorkspaceResourceProperties{}, wantError: "no Prometheus query endpoint"},
		{
			name:       "missing endpoint",
			properties: &armmonitor.AzureMonitorWorkspaceResourceProperties{Metrics: &armmonitor.AzureMonitorWorkspaceMetrics{}},
			wantError:  "no Prometheus query endpoint",
		},
		{
			name: "empty endpoint",
			properties: &armmonitor.AzureMonitorWorkspaceResourceProperties{
				Metrics: &armmonitor.AzureMonitorWorkspaceMetrics{PrometheusQueryEndpoint: to.Ptr("")},
			},
			wantError: "no Prometheus query endpoint",
		},
		{name: "read failure", readError: readError, wantError: "workspace read failed"},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := &monitorfake.AzureMonitorWorkspacesServer{
				Get: func(ctx context.Context, group, name string, options *armmonitor.AzureMonitorWorkspacesClientGetOptions) (fake.Responder[armmonitor.AzureMonitorWorkspacesClientGetResponse], fake.ErrorResponder) {
					require.Equal(t, "metrics-group", group)
					require.Equal(t, "workspace", name)
					var response fake.Responder[armmonitor.AzureMonitorWorkspacesClientGetResponse]
					var failure fake.ErrorResponder
					if test.readError != nil {
						failure.SetError(test.readError)
						return response, failure
					}
					response.SetResponse(http.StatusOK, armmonitor.AzureMonitorWorkspacesClientGetResponse{
						AzureMonitorWorkspaceResource: armmonitor.AzureMonitorWorkspaceResource{Properties: test.properties},
					}, nil)
					return response, failure
				},
			}
			client, err := NewMonitorWorkspaceClient("subscription", &fake.TokenCredential{}, &arm.ClientOptions{
				ClientOptions: policy.ClientOptions{Transport: monitorfake.NewAzureMonitorWorkspacesServerTransport(server), Retry: policy.RetryOptions{MaxRetries: -1}},
			})
			require.NoError(t, err)
			got, err := client.GetPrometheusQueryEndpoint(t.Context(), "metrics-group", "workspace")
			if test.wantError != "" {
				require.ErrorContains(t, err, test.wantError)
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, test.want, got)
			if test.readError != nil {
				require.ErrorIs(t, err, test.readError)
			}
		})
	}
}

func TestNewKustoClusterQueryRequestScopesSubscription(t *testing.T) {
	format := armresourcegraph.ResultFormatObjectArray
	request, err := newKustoClusterQueryRequest("subscription-id", "int-one", format, nil)
	require.NoError(t, err)

	require.Len(t, request.Subscriptions, 1)
	require.NotNil(t, request.Subscriptions[0])
	assert.Equal(t, "subscription-id", *request.Subscriptions[0])
	require.NotNil(t, request.Query)
	assert.Contains(t, *request.Query, "tostring(tags['aroHCPEnvironment']) =~ 'int-one'")
	assert.NotContains(t, *request.Query, "provisioningState) =~ 'Succeeded'")
	assert.Contains(t, *request.Query, "provisioningState = tostring(properties.provisioningState)")
	assert.Contains(t, *request.Query, "| order by id asc")
}

func TestNewKustoClusterQueryRequestRejectsUnsafeEnvironment(t *testing.T) {
	format := armresourcegraph.ResultFormatObjectArray
	_, err := newKustoClusterQueryRequest("subscription-id", "int' | take 1", format, nil)
	assert.ErrorContains(t, err, "only ASCII letters, numbers, and hyphens")
}
