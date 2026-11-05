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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
)

func TestCreateRevisionConfigMap_GetNonNotFoundError(t *testing.T) {
	client := fake.NewSimpleClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "aks-istio-system"}},
	)
	client.PrependReactor("get", "configmaps", func(action k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, fmt.Errorf("etcd connection refused")
	})

	kubeClient := NewKubeClientFromInterface(client)
	err := CreateRevisionConfigMap(context.Background(), kubeClient, "asm-1-29")
	assert.ErrorContains(t, err, "failed to get ConfigMap")
	assert.ErrorContains(t, err, "etcd connection refused")
}

func TestDeleteRevisionConfigMap_NonNotFoundError(t *testing.T) {
	client := fake.NewSimpleClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "aks-istio-system"}},
	)
	client.PrependReactor("delete", "configmaps", func(action k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, fmt.Errorf("etcd unavailable")
	})

	kubeClient := NewKubeClientFromInterface(client)
	err := DeleteRevisionConfigMap(context.Background(), kubeClient, "asm-1-29")
	assert.ErrorContains(t, err, "failed to delete ConfigMap")
	assert.ErrorContains(t, err, "etcd unavailable")
}

func TestCreateRevisionConfigMap_Idempotent(t *testing.T) {
	client := fake.NewSimpleClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "aks-istio-system"}},
	)
	kubeClient := NewKubeClientFromInterface(client)

	err := CreateRevisionConfigMap(context.Background(), kubeClient, "asm-1-29")
	require.NoError(t, err)

	// Second call should be a no-op (same data)
	err = CreateRevisionConfigMap(context.Background(), kubeClient, "asm-1-29")
	require.NoError(t, err)

	cm, err := client.CoreV1().ConfigMaps("aks-istio-system").Get(context.Background(), "istio-shared-configmap-asm-1-29", metav1.GetOptions{})
	require.NoError(t, err)
	assert.Equal(t, "asm-1-29", cm.Labels["istio.io/rev"])
}

func TestDeleteRevisionConfigMap_NotFound(t *testing.T) {
	client := fake.NewSimpleClientset()
	kubeClient := NewKubeClientFromInterface(client)

	err := DeleteRevisionConfigMap(context.Background(), kubeClient, "asm-1-28")
	require.NoError(t, err, "deleting a non-existent ConfigMap should be a no-op")
}

func TestCreateRevisionConfigMap_UpdateNilLabels(t *testing.T) {
	client := fake.NewSimpleClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "aks-istio-system"}},
		&corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Name: "istio-shared-configmap-asm-1-29", Namespace: "aks-istio-system"},
			Data:       map[string]string{"mesh": "stale-data"},
		},
	)
	kubeClient := NewKubeClientFromInterface(client)

	err := CreateRevisionConfigMap(context.Background(), kubeClient, "asm-1-29")
	require.NoError(t, err)

	cm, err := client.CoreV1().ConfigMaps("aks-istio-system").Get(context.Background(), "istio-shared-configmap-asm-1-29", metav1.GetOptions{})
	require.NoError(t, err)
	assert.Equal(t, "asm-1-29", cm.Labels["istio.io/rev"])
}

func TestCreateRevisionConfigMap_UpdateError(t *testing.T) {
	client := fake.NewSimpleClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "aks-istio-system"}},
		&corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Name: "istio-shared-configmap-asm-1-29", Namespace: "aks-istio-system"},
			Data:       map[string]string{"mesh": "stale-data"},
		},
	)
	client.PrependReactor("update", "configmaps", func(action k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, fmt.Errorf("conflict on update")
	})

	kubeClient := NewKubeClientFromInterface(client)
	err := CreateRevisionConfigMap(context.Background(), kubeClient, "asm-1-29")
	assert.ErrorContains(t, err, "failed to update ConfigMap")
	assert.ErrorContains(t, err, "conflict on update")
}

func TestCreateRevisionConfigMap_CreateError(t *testing.T) {
	client := fake.NewSimpleClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "aks-istio-system"}},
	)
	client.PrependReactor("create", "configmaps", func(action k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, fmt.Errorf("quota exceeded")
	})

	kubeClient := NewKubeClientFromInterface(client)
	err := CreateRevisionConfigMap(context.Background(), kubeClient, "asm-1-29")
	assert.ErrorContains(t, err, "failed to create ConfigMap")
	assert.ErrorContains(t, err, "quota exceeded")
}

func sharedConfigMap(revision string) *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      revisionConfigMapName(revision),
			Namespace: istioSystemNamespace,
			Labels:    map[string]string{"istio.io/rev": revision},
		},
	}
}

func TestReconcileOrphanedRevisionConfigMaps(t *testing.T) {
	t.Run("removes only orphaned shared revision ConfigMaps", func(t *testing.T) {
		client := fake.NewSimpleClientset(
			&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: istioSystemNamespace}},
			sharedConfigMap("asm-1-28"),
			sharedConfigMap("asm-1-29"),
			// AKS-managed ConfigMaps also carry istio.io/rev but must never be touched.
			&corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{
				Name: "istio-asm-1-28", Namespace: istioSystemNamespace,
				Labels: map[string]string{"istio.io/rev": "asm-1-28"},
			}},
			&corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{
				Name: "istio-sidecar-injector-asm-1-28", Namespace: istioSystemNamespace,
				Labels: map[string]string{"istio.io/rev": "asm-1-28"},
			}},
		)

		err := ReconcileOrphanedRevisionConfigMaps(
			context.Background(),
			logr.FromContextOrDiscard(context.Background()),
			NewKubeClientFromInterface(client),
			[]string{"asm-1-29"},
		)
		require.NoError(t, err)

		_, err = client.CoreV1().ConfigMaps(istioSystemNamespace).Get(
			context.Background(), "istio-shared-configmap-asm-1-28", metav1.GetOptions{})
		assert.True(t, apierrors.IsNotFound(err), "expected orphaned shared ConfigMap to be removed")

		_, err = client.CoreV1().ConfigMaps(istioSystemNamespace).Get(
			context.Background(), "istio-shared-configmap-asm-1-29", metav1.GetOptions{})
		require.NoError(t, err, "active revision ConfigMap should be preserved")

		for _, name := range []string{"istio-asm-1-28", "istio-sidecar-injector-asm-1-28"} {
			_, err = client.CoreV1().ConfigMaps(istioSystemNamespace).Get(
				context.Background(), name, metav1.GetOptions{})
			require.NoError(t, err, "AKS-managed ConfigMap %q must never be touched", name)
		}
	})

	t.Run("list error is returned to caller", func(t *testing.T) {
		client := fake.NewSimpleClientset(
			&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: istioSystemNamespace}},
		)
		client.PrependReactor("list", "configmaps", func(action k8stesting.Action) (bool, runtime.Object, error) {
			return true, nil, fmt.Errorf("apiserver unavailable")
		})

		err := ReconcileOrphanedRevisionConfigMaps(
			context.Background(),
			logr.FromContextOrDiscard(context.Background()),
			NewKubeClientFromInterface(client),
			[]string{"asm-1-29"},
		)
		assert.ErrorContains(t, err, "list Istio shared revision ConfigMaps")
		assert.ErrorContains(t, err, "apiserver unavailable")
	})

	t.Run("delete error is non-fatal and reconciliation continues", func(t *testing.T) {
		client := fake.NewSimpleClientset(
			&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: istioSystemNamespace}},
			sharedConfigMap("asm-1-27"),
			sharedConfigMap("asm-1-28"),
		)
		client.PrependReactor("delete", "configmaps", func(action k8stesting.Action) (bool, runtime.Object, error) {
			deleteAction := action.(k8stesting.DeleteAction)
			if deleteAction.GetName() == "istio-shared-configmap-asm-1-27" {
				return true, nil, fmt.Errorf("etcd connection refused")
			}
			return false, nil, nil
		})

		err := ReconcileOrphanedRevisionConfigMaps(
			context.Background(),
			logr.FromContextOrDiscard(context.Background()),
			NewKubeClientFromInterface(client),
			[]string{"asm-1-29"},
		)
		require.NoError(t, err)

		_, err = client.CoreV1().ConfigMaps(istioSystemNamespace).Get(
			context.Background(), "istio-shared-configmap-asm-1-27", metav1.GetOptions{})
		require.NoError(t, err, "failed delete should leave ConfigMap in place")

		_, err = client.CoreV1().ConfigMaps(istioSystemNamespace).Get(
			context.Background(), "istio-shared-configmap-asm-1-28", metav1.GetOptions{})
		assert.True(t, apierrors.IsNotFound(err), "reconciliation should continue after non-fatal delete error")
	})
}
