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
	"testing"

	"github.com/go-logr/logr"
	"github.com/go-logr/logr/testr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	coordinationv1 "k8s.io/api/coordination/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
)

func gatewayLease(name string) *coordinationv1.Lease {
	return &coordinationv1.Lease{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: istioSystemNamespace},
	}
}

func TestReconcileOrphanedGatewayLeases(t *testing.T) {
	t.Run("removes only orphaned gateway lease formats", func(t *testing.T) {
		client := fake.NewSimpleClientset(
			&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: istioSystemNamespace}},
			gatewayLease("istio-gateway-deployment-asm-1-28"),
			gatewayLease("istio-gateway-status-leader-asm-1-28"),
			gatewayLease("istio-gateway-deployment-asm-1-29"),
			gatewayLease("some-other-lease"),
		)

		err := ReconcileOrphanedGatewayLeases(
			context.Background(),
			logr.FromContextOrDiscard(context.Background()),
			NewKubeClientFromInterface(client),
			[]string{"asm-1-29"},
		)
		require.NoError(t, err)

		for _, name := range []string{
			"istio-gateway-deployment-asm-1-28",
			"istio-gateway-status-leader-asm-1-28",
		} {
			_, err = client.CoordinationV1().Leases(istioSystemNamespace).Get(
				context.Background(), name, metav1.GetOptions{})
			assert.True(t, apierrors.IsNotFound(err), "expected orphaned lease %q to be removed", name)
		}

		_, err = client.CoordinationV1().Leases(istioSystemNamespace).Get(
			context.Background(), "istio-gateway-deployment-asm-1-29", metav1.GetOptions{})
		require.NoError(t, err)

		_, err = client.CoordinationV1().Leases(istioSystemNamespace).Get(
			context.Background(), "some-other-lease", metav1.GetOptions{})
		require.NoError(t, err)
	})

	t.Run("list error is returned to caller", func(t *testing.T) {
		client := fake.NewSimpleClientset(
			&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: istioSystemNamespace}},
		)
		client.PrependReactor("list", "leases", func(action k8stesting.Action) (bool, runtime.Object, error) {
			return true, nil, fmt.Errorf("apiserver unavailable")
		})

		err := ReconcileOrphanedGatewayLeases(
			context.Background(),
			logr.FromContextOrDiscard(context.Background()),
			NewKubeClientFromInterface(client),
			[]string{"asm-1-29"},
		)
		assert.ErrorContains(t, err, "list Istio gateway leader-election leases")
		assert.ErrorContains(t, err, "apiserver unavailable")
	})

	t.Run("delete NotFound is ignored", func(t *testing.T) {
		client := fake.NewSimpleClientset(
			&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: istioSystemNamespace}},
			gatewayLease("istio-gateway-deployment-asm-1-28"),
		)
		client.PrependReactor("delete", "leases", func(action k8stesting.Action) (bool, runtime.Object, error) {
			deleteAction := action.(k8stesting.DeleteAction)
			return true, nil, apierrors.NewNotFound(coordinationv1.Resource("leases"), deleteAction.GetName())
		})

		err := ReconcileOrphanedGatewayLeases(
			context.Background(),
			logr.FromContextOrDiscard(context.Background()),
			NewKubeClientFromInterface(client),
			[]string{"asm-1-29"},
		)
		require.NoError(t, err)
	})

	t.Run("delete error is non-fatal and reconciliation continues", func(t *testing.T) {
		client := fake.NewSimpleClientset(
			&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: istioSystemNamespace}},
			gatewayLease("istio-gateway-deployment-asm-1-28"),
			gatewayLease("istio-gateway-status-leader-asm-1-28"),
		)
		client.PrependReactor("delete", "leases", func(action k8stesting.Action) (bool, runtime.Object, error) {
			deleteAction := action.(k8stesting.DeleteAction)
			if deleteAction.GetName() == "istio-gateway-deployment-asm-1-28" {
				return true, nil, fmt.Errorf("etcd connection refused")
			}
			return false, nil, nil
		})

		err := ReconcileOrphanedGatewayLeases(
			context.Background(),
			logr.FromContextOrDiscard(context.Background()),
			NewKubeClientFromInterface(client),
			[]string{"asm-1-29"},
		)
		require.NoError(t, err)

		_, err = client.CoordinationV1().Leases(istioSystemNamespace).Get(
			context.Background(), "istio-gateway-deployment-asm-1-28", metav1.GetOptions{})
		require.NoError(t, err, "failed delete should leave lease in place")

		_, err = client.CoordinationV1().Leases(istioSystemNamespace).Get(
			context.Background(), "istio-gateway-status-leader-asm-1-28", metav1.GetOptions{})
		assert.True(t, apierrors.IsNotFound(err), "reconciliation should continue after non-fatal delete error")
	})

	t.Run("removes orphaned leases when mesh is stable", func(t *testing.T) {
		ctx := logr.NewContext(context.Background(), testr.New(t))
		client := fake.NewSimpleClientset(
			&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: istioSystemNamespace}},
			gatewayLease("istio-gateway-deployment-asm-1-28"),
			gatewayLease("istio-gateway-status-leader-asm-1-28"),
			gatewayLease("istio-gateway-deployment-asm-1-29"),
		)
		aks := &fakeAKSClient{
			clusterInfo: &ClusterInfo{ProvisioningState: "Succeeded"},
			meshProfile: &MeshProfile{Revisions: []string{"asm-1-29"}},
			upgradeInfo: &MeshUpgradeInfo{UpgradeInProgress: false},
		}

		reconcileOrphanedGatewayLeases(
			ctx,
			logr.FromContextOrDiscard(ctx),
			aks,
			NewKubeClientFromInterface(client),
			DefaultUpgradeOptions(),
			"asm-1-29",
		)

		assert.Equal(t, []string{"GetClusterState", "GetMeshUpgradeTargets"}, aks.calls)

		for _, name := range []string{
			"istio-gateway-deployment-asm-1-28",
			"istio-gateway-status-leader-asm-1-28",
		} {
			_, err := client.CoordinationV1().Leases(istioSystemNamespace).Get(
				context.Background(), name, metav1.GetOptions{})
			assert.True(t, apierrors.IsNotFound(err), "expected orphaned lease %q to be removed", name)
		}

		_, err := client.CoordinationV1().Leases(istioSystemNamespace).Get(
			context.Background(), "istio-gateway-deployment-asm-1-29", metav1.GetOptions{})
		require.NoError(t, err, "active revision lease should be preserved")
	})

	t.Run("skips while mesh is not stable", func(t *testing.T) {
		tests := []struct {
			name        string
			clusterInfo *ClusterInfo
			meshProfile *MeshProfile
			upgradeInfo *MeshUpgradeInfo
			target      string
		}{
			{
				name:        "upgrade in progress",
				clusterInfo: &ClusterInfo{ProvisioningState: "Succeeded"},
				meshProfile: &MeshProfile{Revisions: []string{"asm-1-29"}},
				upgradeInfo: &MeshUpgradeInfo{UpgradeInProgress: true},
				target:      "asm-1-29",
			},
			{
				name:        "cluster still provisioning",
				clusterInfo: &ClusterInfo{ProvisioningState: "Updating"},
				meshProfile: &MeshProfile{Revisions: []string{"asm-1-29"}},
				upgradeInfo: &MeshUpgradeInfo{UpgradeInProgress: false},
				target:      "asm-1-29",
			},
			{
				name:        "mid-canary with two revisions",
				clusterInfo: &ClusterInfo{ProvisioningState: "Succeeded"},
				meshProfile: &MeshProfile{Revisions: []string{"asm-1-28", "asm-1-29"}},
				upgradeInfo: &MeshUpgradeInfo{UpgradeInProgress: false},
				target:      "asm-1-29",
			},
			{
				name:        "installed revision does not match target",
				clusterInfo: &ClusterInfo{ProvisioningState: "Succeeded"},
				meshProfile: &MeshProfile{Revisions: []string{"asm-1-28"}},
				upgradeInfo: &MeshUpgradeInfo{UpgradeInProgress: false},
				target:      "asm-1-29",
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				ctx := logr.NewContext(context.Background(), testr.New(t))
				client := fake.NewSimpleClientset(
					&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: istioSystemNamespace}},
					gatewayLease("istio-gateway-deployment-asm-1-28"),
				)

				reconcileOrphanedGatewayLeases(
					ctx,
					logr.FromContextOrDiscard(ctx),
					&fakeAKSClient{
						clusterInfo: tt.clusterInfo,
						meshProfile: tt.meshProfile,
						upgradeInfo: tt.upgradeInfo,
					},
					NewKubeClientFromInterface(client),
					DefaultUpgradeOptions(),
					tt.target,
				)

				_, err := client.CoordinationV1().Leases(istioSystemNamespace).Get(
					context.Background(), "istio-gateway-deployment-asm-1-28", metav1.GetOptions{})
				require.NoError(t, err, "orphaned lease should be preserved while mesh is unstable")
			})
		}
	})
}
