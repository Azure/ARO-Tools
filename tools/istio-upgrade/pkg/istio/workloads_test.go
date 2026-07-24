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
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
	"k8s.io/utils/ptr"
)

func TestMigrateWorkloads_TagError(t *testing.T) {
	fakeClient := fake.NewSimpleClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "aks-istio-system"}},
	)
	fakeClient.PrependReactor("get", "mutatingwebhookconfigurations", func(action k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, fmt.Errorf("webhook API unavailable")
	})

	kubeClient := NewKubeClientFromInterface(fakeClient)
	opts := UpgradeOptions{Tag: "prod-stable", RolloutTimeout: time.Second, RolloutPollInterval: time.Millisecond}
	err := migrateWorkloads(context.Background(), kubeClient, opts, "asm-1-29")
	assert.ErrorContains(t, err, "failed to flip revision tag")
	assert.ErrorContains(t, err, "webhook API unavailable")
}

func TestMigrateWorkloads_RolloutError(t *testing.T) {
	fakeClient := fake.NewSimpleClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "aks-istio-system"}},
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "app-ns", Labels: map[string]string{"istio.io/rev": "asm-1-29"}}},
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: "app-ns"},
			Spec:       appsv1.DeploymentSpec{Replicas: ptr.To[int32](1)},
			Status:     appsv1.DeploymentStatus{Replicas: 1, UpdatedReplicas: 0, ReadyReplicas: 0, ObservedGeneration: 1},
		},
	)

	kubeClient := NewKubeClientFromInterface(fakeClient)
	opts := UpgradeOptions{RolloutTimeout: 200 * time.Millisecond, RolloutPollInterval: 50 * time.Millisecond}
	err := migrateWorkloads(context.Background(), kubeClient, opts, "asm-1-29")
	assert.ErrorContains(t, err, "rollout convergence failed")
}

func TestMigrateWorkloads_LabelError(t *testing.T) {
	fakeClient := fake.NewSimpleClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "aks-istio-system"}},
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "app-ns", Labels: map[string]string{"istio.io/rev": "asm-1-28"}}},
	)
	fakeClient.PrependReactor("patch", "namespaces", func(action k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, fmt.Errorf("namespace patch forbidden")
	})

	kubeClient := NewKubeClientFromInterface(fakeClient)
	opts := UpgradeOptions{RolloutTimeout: time.Second, RolloutPollInterval: time.Millisecond}
	err := migrateWorkloads(context.Background(), kubeClient, opts, "asm-1-29")
	assert.ErrorContains(t, err, "failed to update namespace labels")
	assert.ErrorContains(t, err, "namespace patch forbidden")
}

func TestGetMeshNamespaces(t *testing.T) {
	kubeClient := NewKubeClientFromInterface(fake.NewSimpleClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "app-ns", Labels: map[string]string{"istio.io/rev": "asm-1-28"}}},
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "other-ns", Labels: map[string]string{"team": "infra"}}},
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "mesh-ns", Labels: map[string]string{"istio.io/rev": "asm-1-29"}}},
	))

	namespaces, err := kubeClient.GetMeshNamespaces(context.Background())
	require.NoError(t, err)
	assert.Len(t, namespaces, 2)
	revisions := []string{namespaces[0].RevisionLabel, namespaces[1].RevisionLabel}
	assert.ElementsMatch(t, []string{"asm-1-28", "asm-1-29"}, revisions)
}

func TestGetControlPlaneStatus(t *testing.T) {
	kubeClient := NewKubeClientFromInterface(fake.NewSimpleClientset(
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: "istiod-asm-1-28", Namespace: "aks-istio-system"},
			Spec:       appsv1.DeploymentSpec{Replicas: ptr.To[int32](2)},
			Status:     appsv1.DeploymentStatus{AvailableReplicas: 2},
		},
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: "istiod-asm-1-29", Namespace: "aks-istio-system"},
			Spec:       appsv1.DeploymentSpec{Replicas: ptr.To[int32](2)},
			Status:     appsv1.DeploymentStatus{AvailableReplicas: 1},
		},
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: "other-deploy", Namespace: "aks-istio-system"},
			Spec:       appsv1.DeploymentSpec{Replicas: ptr.To[int32](1)},
			Status:     appsv1.DeploymentStatus{AvailableReplicas: 1},
		},
	))

	status, err := GetControlPlaneStatus(context.Background(), kubeClient)
	require.NoError(t, err)
	assert.Len(t, status, 2)
	assert.True(t, status[0].Ready)
	assert.Equal(t, "asm-1-28", status[0].Revision)
	assert.False(t, status[1].Ready)
	assert.Equal(t, "asm-1-29", status[1].Revision)
}

func TestGetIngressGatewayStatus(t *testing.T) {
	kubeClient := NewKubeClientFromInterface(fake.NewSimpleClientset(
		&corev1.Service{
			ObjectMeta: metav1.ObjectMeta{Name: "aks-istio-ingressgateway-external", Namespace: "aks-istio-ingress"},
			Spec: corev1.ServiceSpec{
				Type:     corev1.ServiceTypeLoadBalancer,
				Selector: map[string]string{"app": "ingress"},
			},
			Status: corev1.ServiceStatus{
				LoadBalancer: corev1.LoadBalancerStatus{
					Ingress: []corev1.LoadBalancerIngress{{IP: "10.0.0.1"}},
				},
			},
		},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "ingress-pod-1", Namespace: "aks-istio-ingress", Labels: map[string]string{"app": "ingress"}},
			Status:     corev1.PodStatus{Conditions: []corev1.PodCondition{{Type: corev1.PodReady, Status: corev1.ConditionTrue}}},
		},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "ingress-pod-2", Namespace: "aks-istio-ingress", Labels: map[string]string{"app": "ingress"}},
			Status:     corev1.PodStatus{Conditions: []corev1.PodCondition{{Type: corev1.PodReady, Status: corev1.ConditionFalse}}},
		},
	))

	statuses, err := GetIngressGatewayStatus(context.Background(), kubeClient)
	require.NoError(t, err)
	require.Len(t, statuses, 1)
	assert.Equal(t, "10.0.0.1", statuses[0].ExternalIP)
	assert.Equal(t, 1, statuses[0].HealthyPods)
}

func TestEnsureIngressAnnotations(t *testing.T) {
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "aks-istio-ingressgateway-external", Namespace: "aks-istio-ingress"},
		Spec:       corev1.ServiceSpec{Type: corev1.ServiceTypeLoadBalancer},
	}
	fakeClient := fake.NewSimpleClientset(svc)
	kubeClient := NewKubeClientFromInterface(fakeClient)

	applied, err := EnsureIngressAnnotations(context.Background(), kubeClient, "my-rg", map[string]string{
		"aks-istio-ingressgateway-external": "my-pip",
	})
	require.NoError(t, err)
	assert.True(t, applied)

	updated, err := fakeClient.CoreV1().Services("aks-istio-ingress").Get(context.Background(), "aks-istio-ingressgateway-external", metav1.GetOptions{})
	require.NoError(t, err)
	assert.Equal(t, "my-rg", updated.Annotations["service.beta.kubernetes.io/azure-load-balancer-resource-group"])
	assert.Equal(t, "my-pip", updated.Annotations["service.beta.kubernetes.io/azure-pip-name"])

	applied2, err := EnsureIngressAnnotations(context.Background(), kubeClient, "my-rg", map[string]string{
		"aks-istio-ingressgateway-external": "my-pip",
	})
	require.NoError(t, err)
	assert.False(t, applied2)
}

func TestExecuteRestart(t *testing.T) {
	fakeClient := fake.NewSimpleClientset(
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name: "bare-pod", Namespace: "app-ns",
				Annotations: map[string]string{"sidecar.istio.io/status": `{"revision":"asm-1-28"}`},
			},
			Status: corev1.PodStatus{Phase: corev1.PodRunning},
		},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name: "bare-pod-current", Namespace: "app-ns",
				Annotations: map[string]string{"sidecar.istio.io/status": `{"revision":"asm-1-29"}`},
			},
			Status: corev1.PodStatus{Phase: corev1.PodRunning},
		},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name: "owned-pod", Namespace: "app-ns",
				OwnerReferences: []metav1.OwnerReference{{Name: "web-rs", Kind: "ReplicaSet", APIVersion: "apps/v1", Controller: ptr.To(true)}},
				Annotations:     map[string]string{"sidecar.istio.io/status": `{"revision":"asm-1-28"}`},
			},
			Status: corev1.PodStatus{Phase: corev1.PodRunning},
		},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name: "current-pod", Namespace: "app-ns",
				OwnerReferences: []metav1.OwnerReference{{Name: "api-rs", Kind: "ReplicaSet", APIVersion: "apps/v1", Controller: ptr.To(true)}},
				Annotations:     map[string]string{"sidecar.istio.io/status": `{"revision":"asm-1-29"}`},
			},
			Status: corev1.PodStatus{Phase: corev1.PodRunning},
		},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name: "cache-0", Namespace: "app-ns",
				OwnerReferences: []metav1.OwnerReference{{Name: "cache", Kind: "StatefulSet", APIVersion: "apps/v1", Controller: ptr.To(true)}},
				Annotations:     map[string]string{"sidecar.istio.io/status": `{"revision":"asm-1-28"}`},
			},
			Status: corev1.PodStatus{Phase: corev1.PodRunning},
		},
		&appsv1.ReplicaSet{
			ObjectMeta: metav1.ObjectMeta{
				Name: "web-rs", Namespace: "app-ns",
				OwnerReferences: []metav1.OwnerReference{{Name: "web", Kind: "Deployment", APIVersion: "apps/v1", Controller: ptr.To(true)}},
			},
		},
		&appsv1.ReplicaSet{
			ObjectMeta: metav1.ObjectMeta{
				Name: "api-rs", Namespace: "app-ns",
				OwnerReferences: []metav1.OwnerReference{{Name: "api", Kind: "Deployment", APIVersion: "apps/v1", Controller: ptr.To(true)}},
			},
		},
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: "app-ns"},
			Spec:       appsv1.DeploymentSpec{Replicas: ptr.To[int32](2)},
		},
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: "api", Namespace: "app-ns"},
			Spec:       appsv1.DeploymentSpec{Replicas: ptr.To[int32](1)},
		},
		&appsv1.StatefulSet{
			ObjectMeta: metav1.ObjectMeta{Name: "cache", Namespace: "app-ns"},
			Spec:       appsv1.StatefulSetSpec{Replicas: ptr.To[int32](1)},
		},
	)

	kubeClient := NewKubeClientFromInterface(fakeClient)

	result, err := executeRestart(context.Background(), kubeClient, "app-ns", "asm-1-29")
	require.NoError(t, err)

	assert.Contains(t, result.Restarted, "pod/bare-pod")
	assert.NotContains(t, result.Restarted, "pod/bare-pod-current")
	assert.Contains(t, result.Restarted, "deployment/web")
	assert.NotContains(t, result.Restarted, "deployment/api")
	assert.Contains(t, result.Restarted, "statefulset/cache")

	pods, err := fakeClient.CoreV1().Pods("app-ns").List(context.Background(), metav1.ListOptions{})
	require.NoError(t, err)
	for _, p := range pods.Items {
		assert.NotEqual(t, "bare-pod", p.Name, "stale bare pod should have been deleted")
	}
}

func TestExecuteRestart_DaemonSet(t *testing.T) {
	kubeClient := NewKubeClientFromInterface(fake.NewSimpleClientset(
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name: "ds-pod", Namespace: "app-ns",
				OwnerReferences: []metav1.OwnerReference{{Name: "logging", Kind: "DaemonSet", APIVersion: "apps/v1", Controller: ptr.To(true)}},
				Annotations:     map[string]string{"sidecar.istio.io/status": `{"revision":"asm-1-28"}`},
			},
			Status: corev1.PodStatus{Phase: corev1.PodRunning},
		},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name: "ds-pod-current", Namespace: "app-ns",
				OwnerReferences: []metav1.OwnerReference{{Name: "logging", Kind: "DaemonSet", APIVersion: "apps/v1", Controller: ptr.To(true)}},
				Annotations:     map[string]string{"sidecar.istio.io/status": `{"revision":"asm-1-29"}`},
			},
			Status: corev1.PodStatus{Phase: corev1.PodRunning},
		},
		&appsv1.DaemonSet{
			ObjectMeta: metav1.ObjectMeta{Name: "logging", Namespace: "app-ns"},
		},
	))

	result, err := executeRestart(context.Background(), kubeClient, "app-ns", "asm-1-29")
	require.NoError(t, err)
	assert.Contains(t, result.Restarted, "daemonset/logging")
}

func TestExecuteRestart_BareReplicaSet(t *testing.T) {
	fakeClient := fake.NewSimpleClientset(
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name: "bare-rs-pod", Namespace: "app-ns",
				OwnerReferences: []metav1.OwnerReference{{Name: "orphan-rs", Kind: "ReplicaSet", APIVersion: "apps/v1", Controller: ptr.To(true)}},
				Annotations:     map[string]string{"sidecar.istio.io/status": `{"revision":"asm-1-28"}`},
			},
			Status: corev1.PodStatus{Phase: corev1.PodRunning},
		},
		&appsv1.ReplicaSet{
			ObjectMeta: metav1.ObjectMeta{Name: "orphan-rs", Namespace: "app-ns"},
		},
	)
	kubeClient := NewKubeClientFromInterface(fakeClient)

	result, err := executeRestart(context.Background(), kubeClient, "app-ns", "asm-1-29")
	require.NoError(t, err)
	assert.Contains(t, result.Restarted, "pod/bare-rs-pod", "bare RS pod should be deleted as orphan")

	pods, err := fakeClient.CoreV1().Pods("app-ns").List(context.Background(), metav1.ListOptions{})
	require.NoError(t, err)
	for _, p := range pods.Items {
		assert.NotEqual(t, "bare-rs-pod", p.Name, "bare RS pod should have been deleted")
	}
}

func TestExecuteRestartAllNamespaces(t *testing.T) {
	kubeClient := NewKubeClientFromInterface(fake.NewSimpleClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "ns-a", Labels: map[string]string{"istio.io/rev": "asm-1-29"}}},
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "ns-b", Labels: map[string]string{"istio.io/rev": "asm-1-29"}}},
		// ns-a: stale pod owned by deployment
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name: "pod-a", Namespace: "ns-a",
				Annotations:     map[string]string{"sidecar.istio.io/status": `{"revision":"asm-1-28"}`},
				OwnerReferences: []metav1.OwnerReference{{Name: "rs-a", Kind: "ReplicaSet", APIVersion: "apps/v1", Controller: ptr.To(true)}},
			},
			Status: corev1.PodStatus{Phase: corev1.PodRunning},
		},
		&appsv1.ReplicaSet{
			ObjectMeta: metav1.ObjectMeta{
				Name: "rs-a", Namespace: "ns-a",
				OwnerReferences: []metav1.OwnerReference{{Name: "deploy-a", Kind: "Deployment", APIVersion: "apps/v1", Controller: ptr.To(true)}},
			},
		},
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: "deploy-a", Namespace: "ns-a"},
			Spec:       appsv1.DeploymentSpec{Replicas: ptr.To[int32](1)},
		},
		// ns-b: already current --no stale pods
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name: "pod-b", Namespace: "ns-b",
				Annotations:     map[string]string{"sidecar.istio.io/status": `{"revision":"asm-1-29"}`},
				OwnerReferences: []metav1.OwnerReference{{Name: "rs-b", Kind: "ReplicaSet", APIVersion: "apps/v1", Controller: ptr.To(true)}},
			},
			Status: corev1.PodStatus{Phase: corev1.PodRunning},
		},
	))

	results, err := ExecuteRestartAllNamespaces(context.Background(), kubeClient, "asm-1-29")
	require.NoError(t, err)

	require.Len(t, results, 2, "should return a result per mesh namespace")

	var restarted int
	for _, r := range results {
		if r.Namespace == "ns-a" {
			assert.Contains(t, r.Restarted, "deployment/deploy-a")
		}
		restarted += len(r.Restarted)
	}
	assert.Equal(t, 1, restarted, "only ns-a had stale workloads to restart")
}

func TestExecuteRestartAllNamespaces_NoNamespaces(t *testing.T) {
	kubeClient := NewKubeClientFromInterface(fake.NewSimpleClientset())

	results, err := ExecuteRestartAllNamespaces(context.Background(), kubeClient, "asm-1-29")
	require.NoError(t, err)
	assert.Empty(t, results, "no mesh namespaces should produce empty results")
}

func TestExecuteRestartAllNamespaces_PartialFailure(t *testing.T) {
	fakeClient := fake.NewSimpleClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "ns-ok", Labels: map[string]string{"istio.io/rev": "asm-1-29"}}},
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "ns-fail", Labels: map[string]string{"istio.io/rev": "asm-1-29"}}},
		// ns-ok: stale bare pod (will succeed)
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name: "bare-ok", Namespace: "ns-ok",
				Annotations: map[string]string{"sidecar.istio.io/status": `{"revision":"asm-1-28"}`},
			},
			Status: corev1.PodStatus{Phase: corev1.PodRunning},
		},
		// ns-fail: stale pod owned by deployment that will fail to patch
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name: "pod-fail", Namespace: "ns-fail",
				Annotations:     map[string]string{"sidecar.istio.io/status": `{"revision":"asm-1-28"}`},
				OwnerReferences: []metav1.OwnerReference{{Name: "rs-fail", Kind: "ReplicaSet", APIVersion: "apps/v1", Controller: ptr.To(true)}},
			},
			Status: corev1.PodStatus{Phase: corev1.PodRunning},
		},
		&appsv1.ReplicaSet{
			ObjectMeta: metav1.ObjectMeta{
				Name: "rs-fail", Namespace: "ns-fail",
				OwnerReferences: []metav1.OwnerReference{{Name: "deploy-fail", Kind: "Deployment", APIVersion: "apps/v1", Controller: ptr.To(true)}},
			},
		},
		// Intentionally no Deployment object for deploy-fail --patch will fail
	)

	fakeClient.PrependReactor("patch", "deployments", func(action k8stesting.Action) (bool, runtime.Object, error) {
		pa := action.(k8stesting.PatchAction)
		if pa.GetNamespace() == "ns-fail" {
			return true, nil, fmt.Errorf("simulated patch failure")
		}
		return false, nil, nil
	})
	kubeClient := NewKubeClientFromInterface(fakeClient)

	results, err := ExecuteRestartAllNamespaces(context.Background(), kubeClient, "asm-1-29")
	assert.Error(t, err, "should return aggregated error from ns-fail")
	assert.ErrorContains(t, err, "ns-fail")

	var foundOK bool
	for _, r := range results {
		if r.Namespace == "ns-ok" {
			foundOK = true
			assert.Contains(t, r.Restarted, "pod/bare-ok", "ns-ok restart should have succeeded")
		}
	}
	assert.True(t, foundOK, "successful namespace result should still be included")
}

func TestWaitForRollout_AllReady(t *testing.T) {
	kubeClient := NewKubeClientFromInterface(fake.NewSimpleClientset(
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: "app-ns", Generation: 1},
			Spec:       appsv1.DeploymentSpec{Replicas: ptr.To[int32](2)},
			Status:     appsv1.DeploymentStatus{ObservedGeneration: 1, UpdatedReplicas: 2, ReadyReplicas: 2},
		},
		&appsv1.StatefulSet{
			ObjectMeta: metav1.ObjectMeta{Name: "cache", Namespace: "app-ns", Generation: 1},
			Spec:       appsv1.StatefulSetSpec{Replicas: ptr.To[int32](1)},
			Status:     appsv1.StatefulSetStatus{ObservedGeneration: 1, UpdatedReplicas: 1, ReadyReplicas: 1},
		},
	))

	err := WaitForRollout(context.Background(), kubeClient, "app-ns", 5*time.Second, 100*time.Millisecond)
	require.NoError(t, err)
}

func TestWaitForRollout_SkipsInjectFalse(t *testing.T) {
	kubeClient := NewKubeClientFromInterface(fake.NewSimpleClientset(
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: "no-sidecar", Namespace: "app-ns", Generation: 1},
			Spec: appsv1.DeploymentSpec{
				Replicas: ptr.To[int32](1),
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{"sidecar.istio.io/inject": "false"},
					},
				},
			},
			Status: appsv1.DeploymentStatus{ObservedGeneration: 1, UpdatedReplicas: 0, ReadyReplicas: 0},
		},
	))

	err := WaitForRollout(context.Background(), kubeClient, "app-ns", 5*time.Second, 100*time.Millisecond)
	require.NoError(t, err, "inject-false deployment should be skipped even when not ready")
}

func TestWaitForRollout_SkipsZeroReplicas(t *testing.T) {
	kubeClient := NewKubeClientFromInterface(fake.NewSimpleClientset(
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: "scaled-down", Namespace: "app-ns", Generation: 1},
			Spec:       appsv1.DeploymentSpec{Replicas: ptr.To[int32](0)},
			Status:     appsv1.DeploymentStatus{ObservedGeneration: 1, UpdatedReplicas: 0, ReadyReplicas: 0},
		},
	))

	err := WaitForRollout(context.Background(), kubeClient, "app-ns", 5*time.Second, 100*time.Millisecond)
	require.NoError(t, err, "zero-replica deployment should be skipped")
}

func TestWaitForRollout_DaemonSetReady(t *testing.T) {
	kubeClient := NewKubeClientFromInterface(fake.NewSimpleClientset(
		&appsv1.DaemonSet{
			ObjectMeta: metav1.ObjectMeta{Name: "logging", Namespace: "app-ns", Generation: 1},
			Status: appsv1.DaemonSetStatus{
				ObservedGeneration:     1,
				DesiredNumberScheduled: 3,
				UpdatedNumberScheduled: 3,
				NumberReady:            3,
			},
		},
	))

	err := WaitForRollout(context.Background(), kubeClient, "app-ns", 5*time.Second, 100*time.Millisecond)
	require.NoError(t, err)
}

func TestWaitForRollout_DaemonSetStuck(t *testing.T) {
	kubeClient := NewKubeClientFromInterface(fake.NewSimpleClientset(
		&appsv1.DaemonSet{
			ObjectMeta: metav1.ObjectMeta{Name: "logging", Namespace: "app-ns", Generation: 2},
			Status: appsv1.DaemonSetStatus{
				ObservedGeneration:     1,
				DesiredNumberScheduled: 3,
				UpdatedNumberScheduled: 1,
				NumberReady:            1,
			},
		},
	))

	err := WaitForRollout(context.Background(), kubeClient, "app-ns", 200*time.Millisecond, 50*time.Millisecond)
	assert.Error(t, err)
	assert.ErrorContains(t, err, "rollout did not converge in app-ns")
}

func TestWaitForRollout_Timeout(t *testing.T) {
	kubeClient := NewKubeClientFromInterface(fake.NewSimpleClientset(
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: "stuck", Namespace: "app-ns", Generation: 2},
			Spec:       appsv1.DeploymentSpec{Replicas: ptr.To[int32](1)},
			Status:     appsv1.DeploymentStatus{ObservedGeneration: 1, UpdatedReplicas: 0, ReadyReplicas: 0},
		},
	))

	err := WaitForRollout(context.Background(), kubeClient, "app-ns", 200*time.Millisecond, 50*time.Millisecond)
	assert.Error(t, err)
	assert.ErrorContains(t, err, "rollout did not converge in app-ns")
}

func TestCreateRevisionConfigMap(t *testing.T) {
	fakeClient := fake.NewSimpleClientset()
	kubeClient := NewKubeClientFromInterface(fakeClient)

	err := CreateRevisionConfigMap(context.Background(), kubeClient, "asm-1-29")
	require.NoError(t, err)

	cm, err := fakeClient.CoreV1().ConfigMaps("aks-istio-system").Get(context.Background(), "istio-shared-configmap-asm-1-29", metav1.GetOptions{})
	require.NoError(t, err)
	assert.Equal(t, "asm-1-29", cm.Labels["istio.io/rev"])
	assert.Contains(t, cm.Data["mesh"], "ext-authz")

	err = CreateRevisionConfigMap(context.Background(), kubeClient, "asm-1-29")
	require.NoError(t, err)
}

func TestCreateRevisionConfigMap_UpdateDoesNotMutateFetchedObject(t *testing.T) {
	fakeClient := fake.NewSimpleClientset(&corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "istio-shared-configmap-asm-1-29",
			Namespace: "aks-istio-system",
			Labels: map[string]string{
				"istio.io/rev":                 "asm-1-29",
				"app.kubernetes.io/managed-by": "Helm",
				"helm.sh/chart":                "istio-config-0.1.0",
			},
		},
		Data: map[string]string{"mesh": "old-data"},
	})
	kubeClient := NewKubeClientFromInterface(fakeClient)

	err := CreateRevisionConfigMap(context.Background(), kubeClient, "asm-1-29")
	require.NoError(t, err)

	cm, err := fakeClient.CoreV1().ConfigMaps("aks-istio-system").Get(context.Background(), "istio-shared-configmap-asm-1-29", metav1.GetOptions{})
	require.NoError(t, err)
	assert.Equal(t, "asm-1-29", cm.Labels["istio.io/rev"])
	assert.Equal(t, "Helm", cm.Labels["app.kubernetes.io/managed-by"], "should preserve existing labels")
	assert.Equal(t, "istio-config-0.1.0", cm.Labels["helm.sh/chart"], "should preserve existing labels")
	assert.Contains(t, cm.Data["mesh"], "ext-authz")
}

func TestCreateRevisionConfigMap_UpdatePreservesAnnotations(t *testing.T) {
	fakeClient := fake.NewSimpleClientset(&corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "istio-shared-configmap-asm-1-29",
			Namespace: "aks-istio-system",
			Labels:    map[string]string{"istio.io/rev": "asm-1-29"},
			Annotations: map[string]string{
				"kubectl.kubernetes.io/last-applied-configuration": "{}",
				"custom-annotation": "keep-me",
			},
		},
		Data: map[string]string{"mesh": "old-data"},
	})
	kubeClient := NewKubeClientFromInterface(fakeClient)

	err := CreateRevisionConfigMap(context.Background(), kubeClient, "asm-1-29")
	require.NoError(t, err)

	cm, err := fakeClient.CoreV1().ConfigMaps("aks-istio-system").Get(context.Background(), "istio-shared-configmap-asm-1-29", metav1.GetOptions{})
	require.NoError(t, err)
	assert.Equal(t, "{}", cm.Annotations["kubectl.kubernetes.io/last-applied-configuration"], "should preserve existing annotations")
	assert.Equal(t, "keep-me", cm.Annotations["custom-annotation"], "should preserve existing annotations")
}

func TestDeleteRevisionConfigMap(t *testing.T) {
	fakeClient := fake.NewSimpleClientset(&corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "istio-shared-configmap-asm-1-28",
			Namespace: "aks-istio-system",
			Labels:    map[string]string{"istio.io/rev": "asm-1-28"},
		},
		Data: map[string]string{"mesh": "test"},
	})
	kubeClient := NewKubeClientFromInterface(fakeClient)

	err := DeleteRevisionConfigMap(context.Background(), kubeClient, "asm-1-28")
	require.NoError(t, err)

	_, err = fakeClient.CoreV1().ConfigMaps("aks-istio-system").Get(context.Background(), "istio-shared-configmap-asm-1-28", metav1.GetOptions{})
	assert.True(t, apierrors.IsNotFound(err), "ConfigMap should be deleted")
}

func TestDeleteRevisionConfigMap_NotFoundIsNoop(t *testing.T) {
	kubeClient := NewKubeClientFromInterface(fake.NewSimpleClientset())

	err := DeleteRevisionConfigMap(context.Background(), kubeClient, "asm-1-28")
	require.NoError(t, err, "deleting a non-existent ConfigMap should not error")
}

func TestWaitForRolloutAllNamespaces_ConcurrentSuccess(t *testing.T) {
	kubeClient := NewKubeClientFromInterface(fake.NewSimpleClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "ns-a", Labels: map[string]string{"istio.io/rev": "asm-1-29"}}},
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "ns-b", Labels: map[string]string{"istio.io/rev": "asm-1-29"}}},
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "ns-c", Labels: map[string]string{"istio.io/rev": "asm-1-29"}}},
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: "ns-a", Generation: 1},
			Spec:       appsv1.DeploymentSpec{Replicas: ptr.To[int32](2)},
			Status:     appsv1.DeploymentStatus{ObservedGeneration: 1, UpdatedReplicas: 2, ReadyReplicas: 2},
		},
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: "api", Namespace: "ns-b", Generation: 1},
			Spec:       appsv1.DeploymentSpec{Replicas: ptr.To[int32](3)},
			Status:     appsv1.DeploymentStatus{ObservedGeneration: 1, UpdatedReplicas: 3, ReadyReplicas: 3},
		},
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: "gate", Namespace: "ns-c", Generation: 1},
			Spec:       appsv1.DeploymentSpec{Replicas: ptr.To[int32](1)},
			Status:     appsv1.DeploymentStatus{ObservedGeneration: 1, UpdatedReplicas: 1, ReadyReplicas: 1},
		},
	))

	err := WaitForRolloutAllNamespaces(context.Background(), kubeClient, 5*time.Second, 100*time.Millisecond)
	require.NoError(t, err)
}

func TestWaitForRolloutAllNamespaces_ConcurrentErrors(t *testing.T) {
	kubeClient := NewKubeClientFromInterface(fake.NewSimpleClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "ns-ok", Labels: map[string]string{"istio.io/rev": "asm-1-29"}}},
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "ns-slow", Labels: map[string]string{"istio.io/rev": "asm-1-29"}}},
		// ns-ok: ready
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: "ns-ok", Generation: 1},
			Spec:       appsv1.DeploymentSpec{Replicas: ptr.To[int32](1)},
			Status:     appsv1.DeploymentStatus{ObservedGeneration: 1, UpdatedReplicas: 1, ReadyReplicas: 1},
		},
		// ns-slow: stuck --will timeout
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: "stuck", Namespace: "ns-slow", Generation: 2},
			Spec:       appsv1.DeploymentSpec{Replicas: ptr.To[int32](1)},
			Status:     appsv1.DeploymentStatus{ObservedGeneration: 1, UpdatedReplicas: 0, ReadyReplicas: 0},
		},
	))

	err := WaitForRolloutAllNamespaces(context.Background(), kubeClient, 200*time.Millisecond, 50*time.Millisecond)
	assert.Error(t, err)
	assert.ErrorContains(t, err, "ns-slow")
}

func TestWaitForRolloutAllNamespaces_ContextCancellation(t *testing.T) {
	kubeClient := NewKubeClientFromInterface(fake.NewSimpleClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "ns-a", Labels: map[string]string{"istio.io/rev": "asm-1-29"}}},
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: "stuck", Namespace: "ns-a", Generation: 2},
			Spec:       appsv1.DeploymentSpec{Replicas: ptr.To[int32](1)},
			Status:     appsv1.DeploymentStatus{ObservedGeneration: 1, UpdatedReplicas: 0, ReadyReplicas: 0},
		},
	))

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := WaitForRolloutAllNamespaces(ctx, kubeClient, 5*time.Second, 50*time.Millisecond)
	assert.Error(t, err, "should fail promptly when context is already cancelled")
}

func TestExecuteRestartAllNamespaces_ConcurrentSuccess(t *testing.T) {
	kubeClient := NewKubeClientFromInterface(fake.NewSimpleClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "ns-a", Labels: map[string]string{"istio.io/rev": "asm-1-29"}}},
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "ns-b", Labels: map[string]string{"istio.io/rev": "asm-1-29"}}},
		// ns-a: stale pod
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name: "pod-a", Namespace: "ns-a",
				Annotations:     map[string]string{"sidecar.istio.io/status": `{"revision":"asm-1-28"}`},
				OwnerReferences: []metav1.OwnerReference{{Name: "rs-a", Kind: "ReplicaSet", APIVersion: "apps/v1", Controller: ptr.To(true)}},
			},
			Status: corev1.PodStatus{Phase: corev1.PodRunning},
		},
		&appsv1.ReplicaSet{
			ObjectMeta: metav1.ObjectMeta{
				Name: "rs-a", Namespace: "ns-a",
				OwnerReferences: []metav1.OwnerReference{{Name: "deploy-a", Kind: "Deployment", APIVersion: "apps/v1", Controller: ptr.To(true)}},
			},
		},
		&appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "deploy-a", Namespace: "ns-a"}, Spec: appsv1.DeploymentSpec{Replicas: ptr.To[int32](1)}},
		// ns-b: stale pod
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name: "pod-b", Namespace: "ns-b",
				Annotations:     map[string]string{"sidecar.istio.io/status": `{"revision":"asm-1-28"}`},
				OwnerReferences: []metav1.OwnerReference{{Name: "rs-b", Kind: "ReplicaSet", APIVersion: "apps/v1", Controller: ptr.To(true)}},
			},
			Status: corev1.PodStatus{Phase: corev1.PodRunning},
		},
		&appsv1.ReplicaSet{
			ObjectMeta: metav1.ObjectMeta{
				Name: "rs-b", Namespace: "ns-b",
				OwnerReferences: []metav1.OwnerReference{{Name: "deploy-b", Kind: "Deployment", APIVersion: "apps/v1", Controller: ptr.To(true)}},
			},
		},
		&appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "deploy-b", Namespace: "ns-b"}, Spec: appsv1.DeploymentSpec{Replicas: ptr.To[int32](1)}},
	))

	results, err := ExecuteRestartAllNamespaces(context.Background(), kubeClient, "asm-1-29")
	require.NoError(t, err)
	require.Len(t, results, 2)

	restartedByNS := map[string][]string{}
	for _, r := range results {
		restartedByNS[r.Namespace] = r.Restarted
	}
	assert.Contains(t, restartedByNS["ns-a"], "deployment/deploy-a")
	assert.Contains(t, restartedByNS["ns-b"], "deployment/deploy-b")
}

func TestUpdateMeshNamespaceLabels(t *testing.T) {
	fakeClient := fake.NewSimpleClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "app-ns", Labels: map[string]string{"istio.io/rev": "asm-1-28"}}},
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "other-ns", Labels: map[string]string{"istio.io/rev": "asm-1-28"}}},
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "already-correct", Labels: map[string]string{"istio.io/rev": "asm-1-29"}}},
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "no-mesh"}},
	)
	kubeClient := NewKubeClientFromInterface(fakeClient)

	updated, err := UpdateMeshNamespaceLabels(context.Background(), kubeClient, "asm-1-29")
	require.NoError(t, err)
	assert.Equal(t, 2, updated)

	ns1, err := fakeClient.CoreV1().Namespaces().Get(context.Background(), "app-ns", metav1.GetOptions{})
	require.NoError(t, err)
	assert.Equal(t, "asm-1-29", ns1.Labels["istio.io/rev"])

	ns2, err := fakeClient.CoreV1().Namespaces().Get(context.Background(), "other-ns", metav1.GetOptions{})
	require.NoError(t, err)
	assert.Equal(t, "asm-1-29", ns2.Labels["istio.io/rev"])

	ns3, err := fakeClient.CoreV1().Namespaces().Get(context.Background(), "no-mesh", metav1.GetOptions{})
	require.NoError(t, err)
	assert.Empty(t, ns3.Labels["istio.io/rev"])
}

func TestNamespaceCacheInvalidation(t *testing.T) {
	fakeClient := fake.NewSimpleClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "ns-a", Labels: map[string]string{"istio.io/rev": "asm-1-28"}}},
	)
	kubeClient := NewKubeClientFromInterface(fakeClient)

	ns1, err := kubeClient.GetMeshNamespaces(context.Background())
	require.NoError(t, err)
	assert.Len(t, ns1, 1)

	_, err = fakeClient.CoreV1().Namespaces().Create(context.Background(), &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: "ns-b", Labels: map[string]string{"istio.io/rev": "asm-1-29"}},
	}, metav1.CreateOptions{})
	require.NoError(t, err)

	stale, err := kubeClient.GetMeshNamespaces(context.Background())
	require.NoError(t, err)
	assert.Len(t, stale, 1, "cached result should be stale")

	kubeClient.InvalidateNamespaceCache()

	fresh, err := kubeClient.GetMeshNamespaces(context.Background())
	require.NoError(t, err)
	assert.Len(t, fresh, 2, "after invalidation should see new namespace")
}

func TestParseSidecarRevisionFromImage(t *testing.T) {
	tests := []struct {
		name    string
		pod     corev1.Pod
		wantRev string
		wantOK  bool
	}{
		{
			name: "valid asm image tag",
			pod: corev1.Pod{
				Spec: corev1.PodSpec{Containers: []corev1.Container{
					{Name: "istio-proxy", Image: "mcr.microsoft.com/oss/istio/proxyv2:asm-1-29-3"},
				}},
			},
			wantRev: "asm-1-29",
			wantOK:  true,
		},
		{
			name: "no istio-proxy container",
			pod: corev1.Pod{
				Spec: corev1.PodSpec{Containers: []corev1.Container{
					{Name: "app", Image: "myapp:latest"},
				}},
			},
			wantOK: false,
		},
		{
			name: "image without tag",
			pod: corev1.Pod{
				Spec: corev1.PodSpec{Containers: []corev1.Container{
					{Name: "istio-proxy", Image: "mcr.microsoft.com/oss/istio/proxyv2"},
				}},
			},
			wantOK: false,
		},
		{
			name: "image with non-asm tag",
			pod: corev1.Pod{
				Spec: corev1.PodSpec{Containers: []corev1.Container{
					{Name: "istio-proxy", Image: "mcr.microsoft.com/oss/istio/proxyv2:1.29.0"},
				}},
			},
			wantOK: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rev, ok := parseSidecarRevisionFromImage(tt.pod)
			assert.Equal(t, tt.wantOK, ok)
			if tt.wantOK {
				assert.Equal(t, tt.wantRev, rev)
			}
		})
	}
}

func TestParseSidecarRevision_FallsBackToImage(t *testing.T) {
	pod := corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{"sidecar.istio.io/status": "invalid-json"},
		},
		Spec: corev1.PodSpec{Containers: []corev1.Container{
			{Name: "istio-proxy", Image: "mcr.microsoft.com/oss/istio/proxyv2:asm-1-29-3"},
		}},
	}
	rev, ok := parseSidecarRevision(pod)
	assert.True(t, ok)
	assert.Equal(t, "asm-1-29", rev)
}

func TestMatchesSelector(t *testing.T) {
	tests := []struct {
		name     string
		labels   map[string]string
		selector map[string]string
		want     bool
	}{
		{
			name:     "nil selector matches nothing",
			labels:   map[string]string{"app": "gw"},
			selector: nil,
			want:     false,
		},
		{
			name:     "empty selector matches nothing",
			labels:   map[string]string{"app": "gw"},
			selector: map[string]string{},
			want:     false,
		},
		{
			name:     "matching selector",
			labels:   map[string]string{"app": "gw", "env": "prod"},
			selector: map[string]string{"app": "gw"},
			want:     true,
		},
		{
			name:     "non-matching selector",
			labels:   map[string]string{"app": "web"},
			selector: map[string]string{"app": "gw"},
			want:     false,
		},
		{
			name:     "nil labels with selector",
			labels:   nil,
			selector: map[string]string{"app": "gw"},
			want:     false,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, matchesSelector(tc.labels, tc.selector), "test case: %s", tc.name)
		})
	}
}

func TestCheckWorkloadPending(t *testing.T) {
	tests := []struct {
		name               string
		kind, workloadName string
		desired, updated   int32
		ready              int32
		gen, observed      int64
		want               string
	}{
		{
			name: "zero desired is not pending",
			kind: "Deployment", workloadName: "app",
			desired: 0, updated: 0, ready: 0, gen: 1, observed: 1,
			want: "",
		},
		{
			name: "generation lag",
			kind: "Deployment", workloadName: "app",
			desired: 2, updated: 2, ready: 2, gen: 3, observed: 2,
			want: "Deployment/app(generation-lag)",
		},
		{
			name: "partial ready",
			kind: "StatefulSet", workloadName: "db",
			desired: 3, updated: 3, ready: 1, gen: 1, observed: 1,
			want: "StatefulSet/db(1/3)",
		},
		{
			name: "fully ready",
			kind: "DaemonSet", workloadName: "agent",
			desired: 5, updated: 5, ready: 5, gen: 2, observed: 2,
			want: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := checkWorkloadPending(tt.kind, tt.workloadName, tt.desired, tt.updated, tt.ready, tt.gen, tt.observed)
			assert.Equal(t, tt.want, got)
		})
	}
}
