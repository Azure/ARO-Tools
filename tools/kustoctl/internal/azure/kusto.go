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
	"encoding/json"
	"fmt"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resourcegraph/armresourcegraph"
)

// KustoCluster represents a discovered Kusto cluster from Azure Resource Graph.
type KustoCluster struct {
	Name     string
	Location string
	URI      string
	ID       string
}

// ResourceGraphKustoDiscoveryClient discovers Kusto clusters using Azure Resource Graph.
type ResourceGraphKustoDiscoveryClient struct {
	client *armresourcegraph.Client
}

// NewResourceGraphKustoDiscoveryClient creates a new discovery client.
func NewResourceGraphKustoDiscoveryClient(cred azcore.TokenCredential, clientOptions *arm.ClientOptions) (*ResourceGraphKustoDiscoveryClient, error) {
	client, err := armresourcegraph.NewClient(cred, clientOptions)
	if err != nil {
		return nil, fmt.Errorf("failed to create Resource Graph client: %w", err)
	}

	return &ResourceGraphKustoDiscoveryClient{
		client: client,
	}, nil
}

// DiscoverKustoClusters returns all Kusto clusters that have the aroHCPPurpose
// tag set. This uses Azure Resource Graph to query across all accessible
// subscriptions, following the same discovery pattern as grafanactl.
func (c *ResourceGraphKustoDiscoveryClient) DiscoverKustoClusters(ctx context.Context) ([]KustoCluster, error) {
	query := "resources | where type =~ 'microsoft.kusto/clusters' | where isnotempty(tags['aroHCPPurpose']) and properties.provisioningState == 'Succeeded' | project name, location, uri=tostring(properties.uri), id"
	format := armresourcegraph.ResultFormatObjectArray

	var clusters []KustoCluster
	var skipToken *string
	for {
		result, err := c.client.Resources(ctx, armresourcegraph.QueryRequest{
			Query: &query,
			Options: &armresourcegraph.QueryRequestOptions{
				ResultFormat: &format,
				SkipToken:    skipToken,
			},
		}, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to query Resource Graph: %w", err)
		}

		rows, ok := result.Data.([]any)
		if !ok {
			raw, _ := json.Marshal(result.Data)
			return nil, fmt.Errorf("unexpected Resource Graph result type: %T (raw: %s)", result.Data, string(raw))
		}

		for _, row := range rows {
			m, ok := row.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("unexpected row type: %T", row)
			}

			cluster := KustoCluster{}
			if v, ok := m["name"].(string); ok {
				cluster.Name = v
			}
			if v, ok := m["location"].(string); ok {
				cluster.Location = v
			}
			if v, ok := m["uri"].(string); ok {
				cluster.URI = v
			}
			if v, ok := m["id"].(string); ok {
				cluster.ID = v
			}

			if cluster.Name == "" || cluster.Location == "" || cluster.URI == "" {
				return nil, fmt.Errorf("discovered cluster has missing fields: name=%q location=%q uri=%q id=%q", cluster.Name, cluster.Location, cluster.URI, cluster.ID)
			}

			clusters = append(clusters, cluster)
		}

		skipToken = result.SkipToken
		if skipToken == nil || *skipToken == "" {
			break
		}
	}

	return clusters, nil
}
