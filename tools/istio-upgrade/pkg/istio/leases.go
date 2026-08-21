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
	"regexp"

	"github.com/go-logr/logr"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var gatewayRevisionLeasePattern = regexp.MustCompile(
	`^istio-gateway-(?:deployment|status-leader)-(asm-\d+-\d+)$`,
)

// ReconcileRetiredGatewayLeases removes AKS-managed Istio gateway
// leader-election leases only when their revision is no longer installed.
// The caller is responsible for confirming the mesh is stable before invoking it.
func ReconcileRetiredGatewayLeases(
	ctx context.Context,
	logger logr.Logger,
	kubeClient *KubeClient,
	installedRevisions []string,
) error {
	installed := make(map[string]struct{}, len(installedRevisions))
	for _, revision := range installedRevisions {
		installed[revision] = struct{}{}
	}

	leases, err := kubeClient.client.CoordinationV1().
		Leases(istioSystemNamespace).
		List(ctx, metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("list Istio gateway leader-election leases: %w", err)
	}

	var orphaned []string
	for _, lease := range leases.Items {
		matches := gatewayRevisionLeasePattern.FindStringSubmatch(lease.Name)
		if matches == nil {
			continue
		}

		revision := matches[1]
		if _, isInstalled := installed[revision]; isInstalled {
			continue
		}

		orphaned = append(orphaned, lease.Name)
		if err := kubeClient.client.CoordinationV1().
			Leases(istioSystemNamespace).
			Delete(ctx, lease.Name, metav1.DeleteOptions{}); err != nil &&
			!apierrors.IsNotFound(err) {
			return fmt.Errorf(
				"delete retired Istio gateway leader-election lease %q: %w",
				lease.Name,
				err,
			)
		}
	}

	if len(orphaned) > 0 {
		logger.Info("Deleted orphaned Istio gateway leases", "count", len(orphaned), "leases", orphaned)
	}

	return nil
}
