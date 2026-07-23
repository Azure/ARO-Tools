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
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
	"k8s.io/utils/ptr"
)

func TestHealthCheck(t *testing.T) {
	tests := []struct {
		name           string
		objects        []runtime.Object
		wantPassed     bool
		wantIssueCount int
		wantContains   []string
	}{
		{
			name: "healthy cluster passes all checks",
			objects: []runtime.Object{
				&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "aks-istio-system"}},
				&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "aks-istio-ingress"}},
				&appsv1.Deployment{
					ObjectMeta: metav1.ObjectMeta{Name: "istiod-asm-1-28", Namespace: "aks-istio-system"},
					Spec:       appsv1.DeploymentSpec{Replicas: ptr.To[int32](2)},
					Status:     appsv1.DeploymentStatus{AvailableReplicas: 2},
				},
				&corev1.Service{
					ObjectMeta: metav1.ObjectMeta{Name: "aks-istio-ingressgateway-external", Namespace: "aks-istio-ingress"},
					Spec:       corev1.ServiceSpec{Type: corev1.ServiceTypeLoadBalancer, Selector: map[string]string{"app": "gw"}},
					Status:     corev1.ServiceStatus{LoadBalancer: corev1.LoadBalancerStatus{Ingress: []corev1.LoadBalancerIngress{{IP: "10.0.0.1"}}}},
				},
				&corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{Name: "gw-pod", Namespace: "aks-istio-ingress", Labels: map[string]string{"app": "gw"}},
					Status:     corev1.PodStatus{Conditions: []corev1.PodCondition{{Type: corev1.PodReady, Status: corev1.ConditionTrue}}},
				},
			},
			wantPassed:     true,
			wantIssueCount: 0,
		},
		{
			name: "unhealthy CP and gateway reports all issues",
			objects: []runtime.Object{
				&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "aks-istio-system"}},
				&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "aks-istio-ingress"}},
				&appsv1.Deployment{
					ObjectMeta: metav1.ObjectMeta{Name: "istiod-asm-1-28", Namespace: "aks-istio-system"},
					Spec:       appsv1.DeploymentSpec{Replicas: ptr.To[int32](2)},
					Status:     appsv1.DeploymentStatus{AvailableReplicas: 0},
				},
				&corev1.Service{
					ObjectMeta: metav1.ObjectMeta{Name: "aks-istio-ingressgateway-external", Namespace: "aks-istio-ingress"},
					Spec:       corev1.ServiceSpec{Type: corev1.ServiceTypeLoadBalancer, Selector: map[string]string{"app": "gw"}},
				},
			},
			wantPassed:     false,
			wantIssueCount: 3,
		},
		{
			name: "no control plane deployments",
			objects: []runtime.Object{
				&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "aks-istio-system"}},
				&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "aks-istio-ingress"}},
				&corev1.Service{
					ObjectMeta: metav1.ObjectMeta{Name: "aks-istio-ingressgateway-external", Namespace: "aks-istio-ingress"},
					Spec:       corev1.ServiceSpec{Type: corev1.ServiceTypeLoadBalancer, Selector: map[string]string{"app": "gw"}},
					Status:     corev1.ServiceStatus{LoadBalancer: corev1.LoadBalancerStatus{Ingress: []corev1.LoadBalancerIngress{{IP: "10.0.0.1"}}}},
				},
				&corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{Name: "gw-pod", Namespace: "aks-istio-ingress", Labels: map[string]string{"app": "gw"}},
					Status:     corev1.PodStatus{Conditions: []corev1.PodCondition{{Type: corev1.PodReady, Status: corev1.ConditionTrue}}},
				},
			},
			wantPassed:   false,
			wantContains: []string{"no istiod deployments found"},
		},
		{
			name: "no ingress gateway services",
			objects: []runtime.Object{
				&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "aks-istio-system"}},
				&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "aks-istio-ingress"}},
				&appsv1.Deployment{
					ObjectMeta: metav1.ObjectMeta{Name: "istiod-asm-1-29", Namespace: "aks-istio-system"},
					Spec:       appsv1.DeploymentSpec{Replicas: ptr.To[int32](2)},
					Status:     appsv1.DeploymentStatus{AvailableReplicas: 2},
				},
			},
			wantPassed:   false,
			wantContains: []string{"no ingress gateway services found"},
		},
		{
			name:           "missing both namespaces",
			objects:        []runtime.Object{},
			wantPassed:     false,
			wantIssueCount: 2,
			wantContains: []string{
				"namespace aks-istio-system does not exist",
				"namespace aks-istio-ingress does not exist",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			kubeClient := NewKubeClientFromInterface(fake.NewSimpleClientset(tt.objects...))
			health, err := HealthCheck(context.Background(), kubeClient)
			require.NoError(t, err, "HealthCheck should not return error for %q", tt.name)
			assert.Equal(t, tt.wantPassed, health.Passed, "unexpected Passed for %q", tt.name)
			if tt.wantIssueCount > 0 {
				assert.Len(t, health.Issues, tt.wantIssueCount, "unexpected issue count for %q", tt.name)
			}
			for _, substr := range tt.wantContains {
				found := false
				for _, issue := range health.Issues {
					if strings.Contains(issue, substr) {
						found = true
						break
					}
				}
				assert.True(t, found, "expected issue containing %q for %q, got %v", substr, tt.name, health.Issues)
			}
		})
	}
}

func TestVerifyUpgrade_Passed(t *testing.T) {
	client := fake.NewSimpleClientset(
		&corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Name: "istio-shared-configmap-asm-1-29", Namespace: "aks-istio-system"},
		},
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "app-ns", Labels: map[string]string{"istio.io/rev": "asm-1-29"}}},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name: "web-pod", Namespace: "app-ns",
				Annotations: map[string]string{"sidecar.istio.io/status": `{"revision":"asm-1-29"}`},
			},
			Status: corev1.PodStatus{Phase: corev1.PodRunning},
		},
	)

	kubeClient := NewKubeClientFromInterface(client)
	v, err := VerifyUpgrade(context.Background(), kubeClient, "asm-1-29", "")
	require.NoError(t, err)
	assert.True(t, v.Passed)
	assert.Empty(t, v.Issues)
}

func TestVerifyUpgrade_TagBased(t *testing.T) {
	client := fake.NewSimpleClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "app-ns", Labels: map[string]string{"istio.io/rev": "prod-stable"}}},
		&corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "istio-shared-configmap-asm-1-29", Namespace: "aks-istio-system"}},
		&admissionregistrationv1.MutatingWebhookConfiguration{
			ObjectMeta: metav1.ObjectMeta{Name: "istio-revision-tag-prod-stable-aks-istio-system"},
			Webhooks: []admissionregistrationv1.MutatingWebhook{{
				Name:                    "rev-tag.istio.io",
				ClientConfig:            admissionregistrationv1.WebhookClientConfig{Service: &admissionregistrationv1.ServiceReference{Name: "istiod-asm-1-29"}},
				AdmissionReviewVersions: []string{"v1"},
				SideEffects: func() *admissionregistrationv1.SideEffectClass {
					s := admissionregistrationv1.SideEffectClassNone
					return &s
				}(),
			}},
		},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name: "web-pod", Namespace: "app-ns",
				Annotations: map[string]string{"sidecar.istio.io/status": `{"revision":"asm-1-29"}`},
			},
			Status: corev1.PodStatus{Phase: corev1.PodRunning},
		},
	)

	kubeClient := NewKubeClientFromInterface(client)
	v, err := VerifyUpgrade(context.Background(), kubeClient, "asm-1-29", "prod-stable")
	require.NoError(t, err)
	assert.True(t, v.Passed, "tag-based namespace label should be accepted: %v", v.Issues)
}

func TestVerifyUpgrade_Failed(t *testing.T) {
	client := fake.NewSimpleClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "app-ns", Labels: map[string]string{"istio.io/rev": "asm-1-28"}}},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name: "stale-pod", Namespace: "app-ns",
				Annotations: map[string]string{"sidecar.istio.io/status": `{"revision":"asm-1-28"}`},
			},
			Status: corev1.PodStatus{Phase: corev1.PodRunning},
		},
	)

	kubeClient := NewKubeClientFromInterface(client)
	v, err := VerifyUpgrade(context.Background(), kubeClient, "asm-1-29", "")
	require.NoError(t, err)
	assert.False(t, v.Passed)
	assert.Len(t, v.Issues, 3)
}

func TestVerifyUpgrade_TagWebhookMissing(t *testing.T) {
	client := fake.NewSimpleClientset(
		&corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "istio-shared-configmap-asm-1-29", Namespace: "aks-istio-system"}},
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "app-ns", Labels: map[string]string{"istio.io/rev": "prod-stable"}}},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name: "web-pod", Namespace: "app-ns",
				Annotations: map[string]string{"sidecar.istio.io/status": `{"revision":"asm-1-29"}`},
			},
			Status: corev1.PodStatus{Phase: corev1.PodRunning},
		},
	)

	kubeClient := NewKubeClientFromInterface(client)
	v, err := VerifyUpgrade(context.Background(), kubeClient, "asm-1-29", "prod-stable")
	require.NoError(t, err)
	assert.False(t, v.Passed, "should fail when tag webhook is missing")
	assert.Contains(t, v.Issues[0], "tag webhook")
}

func TestVerifyUpgrade_TagWebhookWrongTarget(t *testing.T) {
	client := fake.NewSimpleClientset(
		&corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "istio-shared-configmap-asm-1-29", Namespace: "aks-istio-system"}},
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "app-ns", Labels: map[string]string{"istio.io/rev": "prod-stable"}}},
		&admissionregistrationv1.MutatingWebhookConfiguration{
			ObjectMeta: metav1.ObjectMeta{Name: "istio-revision-tag-prod-stable-aks-istio-system"},
			Webhooks: []admissionregistrationv1.MutatingWebhook{{
				Name:                    "rev-tag.istio.io",
				ClientConfig:            admissionregistrationv1.WebhookClientConfig{Service: &admissionregistrationv1.ServiceReference{Name: "istiod-asm-1-28"}},
				AdmissionReviewVersions: []string{"v1"},
				SideEffects: func() *admissionregistrationv1.SideEffectClass {
					s := admissionregistrationv1.SideEffectClassNone
					return &s
				}(),
			}},
		},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name: "web-pod", Namespace: "app-ns",
				Annotations: map[string]string{"sidecar.istio.io/status": `{"revision":"asm-1-29"}`},
			},
			Status: corev1.PodStatus{Phase: corev1.PodRunning},
		},
	)

	kubeClient := NewKubeClientFromInterface(client)
	v, err := VerifyUpgrade(context.Background(), kubeClient, "asm-1-29", "prod-stable")
	require.NoError(t, err)
	assert.False(t, v.Passed, "should fail when tag webhook points at wrong revision")
	assert.Contains(t, v.Issues[0], "istiod-asm-1-28")
}

func TestVerifyUpgrade_TagWebhookNilService(t *testing.T) {
	client := fake.NewSimpleClientset(
		&corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "istio-shared-configmap-asm-1-29", Namespace: "aks-istio-system"}},
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "app-ns", Labels: map[string]string{"istio.io/rev": "prod-stable"}}},
		&admissionregistrationv1.MutatingWebhookConfiguration{
			ObjectMeta: metav1.ObjectMeta{Name: "istio-revision-tag-prod-stable-aks-istio-system"},
			Webhooks: []admissionregistrationv1.MutatingWebhook{{
				Name:                    "rev-tag.istio.io",
				ClientConfig:            admissionregistrationv1.WebhookClientConfig{},
				AdmissionReviewVersions: []string{"v1"},
				SideEffects: func() *admissionregistrationv1.SideEffectClass {
					s := admissionregistrationv1.SideEffectClassNone
					return &s
				}(),
			}},
		},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name: "web-pod", Namespace: "app-ns",
				Annotations: map[string]string{"sidecar.istio.io/status": `{"revision":"asm-1-29"}`},
			},
			Status: corev1.PodStatus{Phase: corev1.PodRunning},
		},
	)

	kubeClient := NewKubeClientFromInterface(client)
	v, err := VerifyUpgrade(context.Background(), kubeClient, "asm-1-29", "prod-stable")
	require.NoError(t, err)
	assert.False(t, v.Passed, "should fail when tag webhook has nil Service")
	assert.Contains(t, v.Issues[0], "no service-based config")
}

func TestCheckOrphanedWorkloads_NoneOrphaned(t *testing.T) {
	client := fake.NewSimpleClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "app-ns", Labels: map[string]string{"istio.io/rev": "asm-1-29"}}},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name: "web-pod", Namespace: "app-ns",
				Annotations: map[string]string{"sidecar.istio.io/status": `{"revision":"asm-1-29"}`},
			},
			Status: corev1.PodStatus{Phase: corev1.PodRunning},
		},
	)

	kubeClient := NewKubeClientFromInterface(client)
	orphaned, err := CheckOrphanedWorkloads(context.Background(), kubeClient, "asm-1-29", []string{"asm-1-28", "asm-1-29"})
	require.NoError(t, err)
	assert.Empty(t, orphaned)
}

func TestCheckOrphanedWorkloads_StalePodsDetected(t *testing.T) {
	client := fake.NewSimpleClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "app-ns", Labels: map[string]string{"istio.io/rev": "asm-1-28"}}},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name: "stale-pod", Namespace: "app-ns",
				Annotations: map[string]string{"sidecar.istio.io/status": `{"revision":"asm-1-28"}`},
			},
			Status: corev1.PodStatus{Phase: corev1.PodRunning},
		},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name: "good-pod", Namespace: "app-ns",
				Annotations: map[string]string{"sidecar.istio.io/status": `{"revision":"asm-1-29"}`},
			},
			Status: corev1.PodStatus{Phase: corev1.PodRunning},
		},
	)

	kubeClient := NewKubeClientFromInterface(client)
	orphaned, err := CheckOrphanedWorkloads(context.Background(), kubeClient, "asm-1-29", []string{"asm-1-28", "asm-1-29"})
	require.NoError(t, err)
	assert.Len(t, orphaned, 1)
	assert.Contains(t, orphaned[0], "stale-pod")
	assert.Contains(t, orphaned[0], "asm-1-28")
}

func TestCheckOrphanedWorkloads_SkipsTerminatingPods(t *testing.T) {
	now := metav1.Now()
	client := fake.NewSimpleClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "app-ns", Labels: map[string]string{"istio.io/rev": "asm-1-28"}}},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name: "terminating-pod", Namespace: "app-ns",
				DeletionTimestamp: &now,
				Finalizers:        []string{"test"},
				Annotations:       map[string]string{"sidecar.istio.io/status": `{"revision":"asm-1-28"}`},
			},
			Status: corev1.PodStatus{Phase: corev1.PodRunning},
		},
	)

	kubeClient := NewKubeClientFromInterface(client)
	orphaned, err := CheckOrphanedWorkloads(context.Background(), kubeClient, "asm-1-29", []string{"asm-1-28", "asm-1-29"})
	require.NoError(t, err)
	assert.Empty(t, orphaned, "terminating pods should not be counted as orphaned")
}

func TestCheckOrphanedWorkloads_NoRetiringRevisions(t *testing.T) {
	client := fake.NewSimpleClientset()

	kubeClient := NewKubeClientFromInterface(client)
	orphaned, err := CheckOrphanedWorkloads(context.Background(), kubeClient, "asm-1-29", []string{"asm-1-29"})
	require.NoError(t, err)
	assert.Empty(t, orphaned)
}

func TestNamespaceExists_NonNotFoundError(t *testing.T) {
	client := fake.NewSimpleClientset()
	client.PrependReactor("get", "namespaces", func(action k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, fmt.Errorf("API server unavailable")
	})

	kubeClient := NewKubeClientFromInterface(client)
	_, err := namespaceExists(context.Background(), kubeClient, "aks-istio-system")
	assert.ErrorContains(t, err, "API server unavailable")
}

func TestVerifyUpgrade_ConfigMapGetError(t *testing.T) {
	client := fake.NewSimpleClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "aks-istio-system"}},
	)
	client.PrependReactor("get", "configmaps", func(action k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, fmt.Errorf("etcd timeout")
	})

	kubeClient := NewKubeClientFromInterface(client)
	_, err := VerifyUpgrade(context.Background(), kubeClient, "asm-1-29", "")
	assert.ErrorContains(t, err, "failed to get ConfigMap")
	assert.ErrorContains(t, err, "etcd timeout")
}

func TestVerifyUpgrade_TagWebhookGetError(t *testing.T) {
	client := fake.NewSimpleClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "aks-istio-system"}},
		&corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Name: "istio-shared-configmap-asm-1-29", Namespace: "aks-istio-system"},
		},
	)
	client.PrependReactor("get", "mutatingwebhookconfigurations", func(action k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, fmt.Errorf("webhook API error")
	})

	kubeClient := NewKubeClientFromInterface(client)
	_, err := VerifyUpgrade(context.Background(), kubeClient, "asm-1-29", "prod-stable")
	assert.ErrorContains(t, err, "failed to get tag webhook")
	assert.ErrorContains(t, err, "webhook API error")
}

func TestCheckOrphanedWorkloads_PodListError(t *testing.T) {
	client := fake.NewSimpleClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "app-ns", Labels: map[string]string{"istio.io/rev": "asm-1-29"}}},
	)
	client.PrependReactor("list", "pods", func(action k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, fmt.Errorf("pod list denied")
	})

	kubeClient := NewKubeClientFromInterface(client)
	_, err := CheckOrphanedWorkloads(context.Background(), kubeClient, "asm-1-29", []string{"asm-1-28", "asm-1-29"})
	assert.ErrorContains(t, err, "pod list denied")
}

func TestHealthCheck_NamespaceCheckError(t *testing.T) {
	client := fake.NewSimpleClientset()
	client.PrependReactor("get", "namespaces", func(action k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, fmt.Errorf("API server down")
	})

	kubeClient := NewKubeClientFromInterface(client)
	_, err := HealthCheck(context.Background(), kubeClient)
	assert.ErrorContains(t, err, "API server down")
}

func TestHealthCheck_ControlPlaneCheckError(t *testing.T) {
	client := fake.NewSimpleClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "aks-istio-system"}},
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "aks-istio-ingress"}},
	)
	client.PrependReactor("list", "deployments", func(action k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, fmt.Errorf("deployment list forbidden")
	})

	kubeClient := NewKubeClientFromInterface(client)
	_, err := HealthCheck(context.Background(), kubeClient)
	assert.ErrorContains(t, err, "control plane check")
	assert.ErrorContains(t, err, "deployment list forbidden")
}

func TestHealthCheck_IngressCheckError(t *testing.T) {
	client := fake.NewSimpleClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "aks-istio-system"}},
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "aks-istio-ingress"}},
	)
	client.PrependReactor("list", "services", func(action k8stesting.Action) (bool, runtime.Object, error) {
		if action.GetNamespace() == "aks-istio-ingress" {
			return true, nil, fmt.Errorf("service list timeout")
		}
		return false, nil, nil
	})

	kubeClient := NewKubeClientFromInterface(client)
	_, err := HealthCheck(context.Background(), kubeClient)
	assert.ErrorContains(t, err, "ingress check")
	assert.ErrorContains(t, err, "service list timeout")
}

func TestVerifyUpgrade_NamespaceListError(t *testing.T) {
	client := fake.NewSimpleClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "aks-istio-system"}},
		&corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Name: "istio-shared-configmap-asm-1-29", Namespace: "aks-istio-system"},
		},
	)
	client.PrependReactor("list", "namespaces", func(action k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, fmt.Errorf("namespace list error")
	})

	kubeClient := NewKubeClientFromInterface(client)
	_, err := VerifyUpgrade(context.Background(), kubeClient, "asm-1-29", "")
	assert.ErrorContains(t, err, "failed to list mesh namespaces")
	assert.ErrorContains(t, err, "namespace list error")
}

func TestVerifyUpgrade_PodListError(t *testing.T) {
	client := fake.NewSimpleClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "aks-istio-system"}},
		&corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Name: "istio-shared-configmap-asm-1-29", Namespace: "aks-istio-system"},
		},
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "app-ns", Labels: map[string]string{"istio.io/rev": "asm-1-29"}}},
	)
	client.PrependReactor("list", "pods", func(action k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, fmt.Errorf("pod list forbidden")
	})

	kubeClient := NewKubeClientFromInterface(client)
	_, err := VerifyUpgrade(context.Background(), kubeClient, "asm-1-29", "")
	assert.ErrorContains(t, err, "failed to verify pods")
	assert.ErrorContains(t, err, "pod list forbidden")
}

func TestVerifyUpgrade_NamespaceLabelMismatchWithoutTag(t *testing.T) {
	client := fake.NewSimpleClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "aks-istio-system"}},
		&corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Name: "istio-shared-configmap-asm-1-29", Namespace: "aks-istio-system"},
		},
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "app-ns", Labels: map[string]string{"istio.io/rev": "asm-1-28"}}},
	)

	kubeClient := NewKubeClientFromInterface(client)
	result, err := VerifyUpgrade(context.Background(), kubeClient, "asm-1-29", "")
	require.NoError(t, err)
	assert.False(t, result.Passed)
	assert.Len(t, result.Issues, 1)
	assert.Contains(t, result.Issues[0], "app-ns has label asm-1-28")
}

func TestVerifyUpgrade_NamespaceLabelMismatchWithTag(t *testing.T) {
	client := fake.NewSimpleClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "aks-istio-system"}},
		&corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Name: "istio-shared-configmap-asm-1-29", Namespace: "aks-istio-system"},
		},
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "app-ns", Labels: map[string]string{"istio.io/rev": "asm-1-27"}}},
		&admissionregistrationv1.MutatingWebhookConfiguration{
			ObjectMeta: metav1.ObjectMeta{Name: "istio-revision-tag-prod-stable-aks-istio-system"},
			Webhooks: []admissionregistrationv1.MutatingWebhook{{
				Name: "rev.namespace.sidecar-injector.istio.io",
				ClientConfig: admissionregistrationv1.WebhookClientConfig{
					Service: &admissionregistrationv1.ServiceReference{Name: "istiod-asm-1-29", Namespace: "aks-istio-system"},
				},
			}},
		},
	)

	kubeClient := NewKubeClientFromInterface(client)
	result, err := VerifyUpgrade(context.Background(), kubeClient, "asm-1-29", "prod-stable")
	require.NoError(t, err)
	assert.False(t, result.Passed)
	require.Len(t, result.Issues, 1)
	assert.Contains(t, result.Issues[0], "asm-1-29 or prod-stable")
}

func TestCheckOrphanedWorkloads_NamespaceListError(t *testing.T) {
	client := fake.NewSimpleClientset()
	client.PrependReactor("list", "namespaces", func(action k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, fmt.Errorf("namespace list denied")
	})

	kubeClient := NewKubeClientFromInterface(client)
	_, err := CheckOrphanedWorkloads(context.Background(), kubeClient, "asm-1-29", []string{"asm-1-28", "asm-1-29"})
	assert.ErrorContains(t, err, "failed to list mesh namespaces")
	assert.ErrorContains(t, err, "namespace list denied")
}
