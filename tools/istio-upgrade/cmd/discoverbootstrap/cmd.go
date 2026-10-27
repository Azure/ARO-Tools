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

package discoverbootstrap

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/containerservice/armcontainerservice/v6"
	"github.com/spf13/cobra"

	"github.com/Azure/ARO-Tools/tools/istio-upgrade/pkg/istio"
)

// NewCommand creates read-only bootstrap discovery. Only ARM managed-cluster
// listing permission is needed; discovery never loads Kubernetes credentials.
func NewCommand() (*cobra.Command, error) {
	return newCommand(newManagedClustersClient)
}

// newManagedClustersClient uses the same Azure credential chain as Istio upgrades.
func newManagedClustersClient(subscriptionID string) (*armcontainerservice.ManagedClustersClient, error) {
	cred, err := azidentity.NewDefaultAzureCredential(&azidentity.DefaultAzureCredentialOptions{RequireAzureTokenCredentials: true})
	if err != nil {
		return nil, fmt.Errorf("failed to create credential: %w", err)
	}
	client, err := armcontainerservice.NewManagedClustersClient(subscriptionID, cred, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create AKS client: %w", err)
	}
	return client, nil
}

// newCommand isolates SDK client construction so command tests can use an HTTP
// transport without real Azure credentials or a Kubernetes connection.
func newCommand(newClient func(string) (*armcontainerservice.ManagedClustersClient, error)) (*cobra.Command, error) {
	var subscriptionID, resourceGroup, clusterName, versions, versionsJSON, outputFile string
	cmd := &cobra.Command{
		Use:           "discover-bootstrap",
		Short:         "Discover creation-only Istio revisions without reading the target AKS resource",
		Args:          cobra.NoArgs,
		SilenceErrors: true,
		SilenceUsage:  true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if strings.TrimSpace(subscriptionID) == "" || strings.TrimSpace(resourceGroup) == "" || strings.TrimSpace(clusterName) == "" {
				return fmt.Errorf("--subscription-id, --resource-group and --cluster-name must be nonempty")
			}
			var targets []string
			if cmd.Flags().Changed("versions-json") {
				if err := json.Unmarshal([]byte(versionsJSON), &targets); err != nil {
					return fmt.Errorf("invalid --versions-json: expected an array of revisions: %w", err)
				}
			} else {
				targets = strings.Split(versions, ",")
			}
			if len(targets) == 0 {
				return fmt.Errorf("at least one bootstrap Istio revision is required")
			}
			for _, target := range targets {
				if !istio.RevisionPattern.MatchString(target) {
					return fmt.Errorf("invalid --versions revision %q: must match %s", target, istio.RevisionPattern.String())
				}
			}
			client, err := newClient(subscriptionID)
			if err != nil {
				return err
			}
			revisions, err := istio.DiscoverBootstrap(cmd.Context(), client, resourceGroup, clusterName, targets)
			if err != nil {
				return err
			}
			data, err := json.Marshal(map[string]string{"istioVersions": strings.Join(revisions, ",")})
			if err != nil {
				return fmt.Errorf("failed to encode discovery output: %w", err)
			}
			data = append(data, '\n')
			if outputFile != "" {
				if err := os.WriteFile(outputFile, data, 0o600); err != nil {
					return fmt.Errorf("failed to write discovery output: %w", err)
				}
			}
			if _, err := cmd.OutOrStdout().Write(data); err != nil {
				return fmt.Errorf("failed to write discovery stdout: %w", err)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&subscriptionID, "subscription-id", "", "Azure subscription ID")
	cmd.Flags().StringVar(&resourceGroup, "resource-group", "", "Azure resource group containing the AKS cluster")
	cmd.Flags().StringVar(&clusterName, "cluster-name", "", "AKS cluster name")
	cmd.Flags().StringVar(&versions, "versions", "", "configured bootstrap Istio revisions as CSV (e.g. asm-1-29)")
	cmd.Flags().StringVar(&versionsJSON, "versions-json", "", "configured bootstrap Istio revisions as a JSON array")
	cmd.Flags().StringVar(&outputFile, "output-file", "", "also write the JSON output to this file")
	for _, name := range []string{"subscription-id", "resource-group", "cluster-name"} {
		if err := cmd.MarkFlagRequired(name); err != nil {
			return nil, fmt.Errorf("failed to mark flag %s as required: %w", name, err)
		}
	}
	cmd.MarkFlagsOneRequired("versions", "versions-json")
	cmd.MarkFlagsMutuallyExclusive("versions", "versions-json")
	return cmd, nil
}
