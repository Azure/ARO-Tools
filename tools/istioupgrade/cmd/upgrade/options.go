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

package upgrade

import (
	"context"

	"github.com/go-logr/logr"
	"github.com/spf13/cobra"
)

type Options struct {
	SubscriptionID string
	ResourceGroup  string
	ClusterName    string
	KubeconfigPath string
	Versions       string
	Tag            string
	DryRun         bool
}

func DefaultOptions() *Options {
	return &Options{}
}

func BindOptions(opts *Options, cmd *cobra.Command) error {
	flags := cmd.Flags()
	flags.StringVar(&opts.SubscriptionID, "subscription-id", "", "Azure subscription ID")
	flags.StringVar(&opts.ResourceGroup, "resource-group", "", "Azure resource group")
	flags.StringVar(&opts.ClusterName, "cluster-name", "", "AKS cluster name")
	flags.StringVar(&opts.KubeconfigPath, "kubeconfig", "", "Path to kubeconfig file")
	flags.StringVar(&opts.Versions, "versions", "", "Target Istio versions (CSV)")
	flags.StringVar(&opts.Tag, "tag", "", "Revision tag name")
	flags.BoolVar(&opts.DryRun, "dry-run", false, "Log actions without executing")

	for _, flag := range []string{"subscription-id", "resource-group", "cluster-name", "versions"} {
		if err := cmd.MarkFlagRequired(flag); err != nil {
			return err
		}
	}
	return nil
}

func (o *Options) Run(ctx context.Context) error {
	logger := logr.FromContextOrDiscard(ctx)

	logger.Info("Istio upgrade step running (Stage 1 — no-op)",
		"cluster", o.ClusterName,
		"versions", o.Versions,
		"dryRun", o.DryRun,
	)

	logger.Info("Istio upgrade step completed (Stage 1 — no action taken)",
		"cluster", o.ClusterName,
		"versions", o.Versions,
	)

	return nil
}
