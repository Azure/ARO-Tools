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
	"github.com/go-logr/logr"
	"github.com/spf13/cobra"
)

// Required permissions:
//
//	ARM: Contributor + Reader on subscription
//	Kubernetes: cluster-admin equivalent (namespaces, configmaps, deployments,
//	  statefulsets, daemonsets, pods, services, mutatingwebhookconfigurations)
func NewCommand() (*cobra.Command, error) {
	opts := DefaultOptions()
	cmd := &cobra.Command{
		Use:           "run",
		Short:         "Run an Istio upgrade against an AKS cluster",
		SilenceErrors: true,
		SilenceUsage:  true,
		RunE: func(cmd *cobra.Command, args []string) error {
			validated, err := opts.Validate()
			if err != nil {
				return err
			}

			ctx := cmd.Context()
			logger := logr.FromContextOrDiscard(ctx)

			completed, err := validated.Complete(logger)
			if err != nil {
				return err
			}

			return completed.Run(ctx)
		},
	}
	if err := BindOptions(opts, cmd); err != nil {
		return nil, err
	}
	return cmd, nil
}
