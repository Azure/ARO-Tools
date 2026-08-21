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
	"testing"

	"github.com/go-logr/logr"
	"github.com/go-logr/logr/testr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	coordinationv1 "k8s.io/api/coordination/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func gatewayLease(name string) *coordinationv1.Lease {
	return &coordinationv1.Lease{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: istioSystemNamespace},
	}
}

func TestReconcileRetiredGatewayLeases(t *testing.T) {
	t.Run("deletes only retired gateway lease formats", func(t *testing.T) {
		client := fake.NewSimpleClientset(
			&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: istioSystemNamespace}},
			gatewayLease("istio-gateway-deployment-asm-1-28"),
			gatewayLease("istio-gateway-status-leader-asm-1-28"),
			gatewayLease("istio-gateway-deployment-asm-1-29"),
			gatewayLease("some-other-lease"),
		)

		err := ReconcileRetiredGatewayLeases(
			context.Background(),
			NewKubeClientFromInterface(client),
			[]string{"asm-1-29"},
		)
		require.NoError(t, err)

		_, err = client.CoordinationV1().Leases(istioSystemNamespace).Get(
			context.Background(), "istio-gateway-deployment-asm-1-28", metav1.GetOptions{})
		assert.True(t, apierrors.IsNotFound(err))

		_, err = client.CoordinationV1().Leases(istioSystemNamespace).Get(
			context.Background(), "istio-gateway-deployment-asm-1-29", metav1.GetOptions{})
		require.NoError(t, err)

		_, err = client.CoordinationV1().Leases(istioSystemNamespace).Get(
			context.Background(), "some-other-lease", metav1.GetOptions{})
		require.NoError(t, err)
	})

	t.Run("skips while mesh is not stable", func(t *testing.T) {
		ctx := logr.NewContext(context.Background(), testr.New(t))
		client := fake.NewSimpleClientset(
			&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: istioSystemNamespace}},
			gatewayLease("istio-gateway-deployment-asm-1-28"),
		)

		err := reconcileRetiredGatewayLeases(
			ctx,
			logr.FromContextOrDiscard(ctx),
			&fakeAKSClient{
				clusterInfo: &ClusterInfo{ProvisioningState: "Succeeded"},
				meshProfile: &MeshProfile{Revisions: []string{"asm-1-29"}},
				upgradeInfo: &MeshUpgradeInfo{UpgradeInProgress: true},
			},
			NewKubeClientFromInterface(client),
			DefaultUpgradeOptions(),
			"asm-1-29",
		)
		require.NoError(t, err)

		_, err = client.CoordinationV1().Leases(istioSystemNamespace).Get(
			context.Background(), "istio-gateway-deployment-asm-1-28", metav1.GetOptions{})
		require.NoError(t, err)
	})
}
