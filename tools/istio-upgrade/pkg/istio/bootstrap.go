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

package istio

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/containerservice/armcontainerservice/v6"
)

// DiscoverBootstrap returns the configured revisions only when the target AKS
// cluster is absent. Every existing cluster, including Failed clusters and active
// canaries, must retain its mesh configuration. Listing avoids the failed-resource
// GET behavior and must finish successfully before absence can be established.
func DiscoverBootstrap(ctx context.Context, client *armcontainerservice.ManagedClustersClient, resourceGroup, clusterName string, versions []string) ([]string, error) {
	if client == nil {
		return nil, fmt.Errorf("AKS client is required")
	}
	if strings.TrimSpace(resourceGroup) == "" || strings.TrimSpace(clusterName) == "" {
		return nil, fmt.Errorf("resource group and cluster name are required")
	}
	if len(versions) == 0 {
		return nil, fmt.Errorf("at least one bootstrap Istio revision is required")
	}
	for _, version := range versions {
		if !RevisionPattern.MatchString(version) {
			return nil, fmt.Errorf("invalid bootstrap Istio revision %q: must match %s", version, RevisionPattern.String())
		}
	}

	exists := false
	pager := client.NewListByResourceGroupPager(resourceGroup, nil)
	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to list AKS clusters in resource group %s: %w", resourceGroup, err)
		}
		for _, cluster := range page.Value {
			if cluster != nil && cluster.Name != nil && strings.EqualFold(*cluster.Name, clusterName) {
				exists = true
			}
		}
	}
	if exists {
		return []string{}, nil
	}
	return slices.Clone(versions), nil
}
