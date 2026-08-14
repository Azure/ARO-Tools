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
	// Environment is the value of the cluster's aroHCPEnvironment tag, used to
	// scope entity-group membership to a single ARO-HCP environment. It may be
	// empty when the cluster has not yet been tagged (for example during an
	// in-progress tag rollout or while Resource Graph indexing lags behind ARM).
	Environment string
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

// kustoDiscoveryQuery builds the Azure Resource Graph query used to discover
// Kusto clusters. Every candidate must carry the aroHCPPurpose tag and be in a
// Succeeded provisioning state. The cluster's aroHCPEnvironment tag is projected
// so the caller can scope entity-group membership per ARO-HCP environment (int,
// stg, prod) and detect clusters that are not yet tagged. Environment scoping is
// performed client-side after discovery (see selectClustersForEnvironment in the
// entitygroups package), so no user-controlled value is ever interpolated into
// the KQL query.
func kustoDiscoveryQuery() string {
	return "resources | where type =~ 'microsoft.kusto/clusters' | where isnotempty(tags['aroHCPPurpose']) and properties.provisioningState == 'Succeeded' | project name, location, uri=tostring(properties.uri), id, environment=tostring(tags['aroHCPEnvironment'])"
}

// DiscoverKustoClusters returns every Kusto cluster that carries the
// aroHCPPurpose tag, using Azure Resource Graph to query across all accessible
// subscriptions (the same discovery pattern as grafanactl). Each returned
// cluster includes its aroHCPEnvironment tag value, which may be empty. Callers
// scope the result to a single environment via selectClustersForEnvironment,
// which fails closed when a discovered cluster is missing a valid
// aroHCPEnvironment tag so membership is never rebuilt from a partial set.
func (c *ResourceGraphKustoDiscoveryClient) DiscoverKustoClusters(ctx context.Context) ([]KustoCluster, error) {
	query := kustoDiscoveryQuery()
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
			if v, ok := m["environment"].(string); ok {
				cluster.Environment = v
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
