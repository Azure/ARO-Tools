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

package run

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	"github.com/spf13/cobra"

	"github.com/Azure/ARO-Tools/tools/istio-upgrade/pkg/istio"
)

// RawOptions holds input values from CLI flags.
type RawOptions struct {
	SubscriptionID string
	ResourceGroup  string
	ClusterName    string
	KubeconfigPath string
	Versions       string
	Tag            string
	IngressIPName  string
	RegionRG       string
	DryRun         bool
	StopAfter      string
}

func DefaultOptions() *RawOptions {
	return &RawOptions{
		DryRun: true,
	}
}

func BindOptions(opts *RawOptions, cmd *cobra.Command) error {
	cmd.Flags().StringVar(&opts.SubscriptionID, "subscription-id", opts.SubscriptionID, "Azure subscription ID")
	cmd.Flags().StringVar(&opts.ResourceGroup, "resource-group", opts.ResourceGroup, "Azure resource group containing the AKS cluster")
	cmd.Flags().StringVar(&opts.ClusterName, "cluster-name", opts.ClusterName, "AKS cluster name")
	cmd.Flags().StringVar(&opts.KubeconfigPath, "kubeconfig", opts.KubeconfigPath, "path to kubeconfig file")
	cmd.Flags().StringVar(&opts.Versions, "versions", opts.Versions, "target Istio revision (e.g. asm-1-29)")
	cmd.Flags().StringVar(&opts.Tag, "tag", opts.Tag, "revision tag name to flip (e.g. 'default')")
	cmd.Flags().StringVar(&opts.IngressIPName, "ingress-ip-name", opts.IngressIPName, "public IP name for ingress gateway annotation")
	cmd.Flags().StringVar(&opts.RegionRG, "region-rg", opts.RegionRG, "resource group containing the ingress public IP")
	cmd.Flags().BoolVar(&opts.DryRun, "dry-run", opts.DryRun, "log the decision without mutating the cluster")
	cmd.Flags().StringVar(&opts.StopAfter, "stop-after", opts.StopAfter, "halt upgrade at a phase for resume testing (canary-start, orphan-check)")

	for _, name := range []string{"subscription-id", "resource-group", "cluster-name", "kubeconfig", "versions"} {
		if err := cmd.MarkFlagRequired(name); err != nil {
			return fmt.Errorf("failed to mark flag %s as required: %w", name, err)
		}
	}
	return nil
}

type validatedOptions struct {
	*RawOptions
}

type ValidatedOptions struct {
	*validatedOptions
}

func (o *RawOptions) Validate() (*ValidatedOptions, error) {
	if o.SubscriptionID == "" {
		return nil, fmt.Errorf("--subscription-id is required")
	}
	if o.ResourceGroup == "" {
		return nil, fmt.Errorf("--resource-group is required")
	}
	if o.ClusterName == "" {
		return nil, fmt.Errorf("--cluster-name is required")
	}
	if o.KubeconfigPath == "" {
		return nil, fmt.Errorf("--kubeconfig is required")
	}
	if o.Versions == "" {
		return nil, fmt.Errorf("--versions is required")
	}
	if !istio.RevisionPattern.MatchString(o.Versions) {
		return nil, fmt.Errorf("invalid --versions %q: must match %s", o.Versions, istio.RevisionPattern.String())
	}
	if (o.IngressIPName == "") != (o.RegionRG == "") {
		return nil, fmt.Errorf("--ingress-ip-name and --region-rg must both be set or both be empty (got ingress-ip-name=%q, region-rg=%q)", o.IngressIPName, o.RegionRG)
	}
	if o.StopAfter != "" {
		if _, err := istio.ValidateStopAfter(o.StopAfter); err != nil {
			return nil, err
		}
	}
	return &ValidatedOptions{
		validatedOptions: &validatedOptions{
			RawOptions: o,
		},
	}, nil
}

type completedOptions struct {
	AKSClient  istio.AKSClusterClient
	KubeClient *istio.KubeClient
	Opts       istio.UpgradeOptions
}

type Options struct {
	*completedOptions
}

func (o *ValidatedOptions) Complete(logger logr.Logger) (*Options, error) {
	aksClient, err := istio.NewAKSClient(o.SubscriptionID, logger, istio.DefaultAKSClientConfig())
	if err != nil {
		return nil, err
	}

	kubeClient, err := istio.NewKubeClient(o.KubeconfigPath)
	if err != nil {
		return nil, err
	}

	upgradeOpts := istio.DefaultUpgradeOptions()
	upgradeOpts.ResourceGroup = o.ResourceGroup
	upgradeOpts.ClusterName = o.ClusterName
	upgradeOpts.Versions = o.Versions
	upgradeOpts.Tag = o.Tag
	upgradeOpts.IngressIPName = o.IngressIPName
	upgradeOpts.RegionRG = o.RegionRG
	upgradeOpts.DryRun = o.DryRun
	if o.StopAfter != "" {
		upgradeOpts.StopAfter = istio.StopAfter(o.StopAfter)
	}

	return &Options{
		completedOptions: &completedOptions{
			AKSClient:  aksClient,
			KubeClient: kubeClient,
			Opts:       upgradeOpts,
		},
	}, nil
}

func (o *Options) Run(ctx context.Context) error {
	return istio.RunUpgrade(ctx, o.Opts, o.AKSClient, o.KubeClient)
}
