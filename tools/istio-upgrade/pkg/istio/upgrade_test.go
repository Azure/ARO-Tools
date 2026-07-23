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

	"github.com/go-logr/logr"
	"github.com/go-logr/logr/testr"
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

type aksCallArgs struct {
	ResourceGroup string
	ClusterName   string
	Revision      string
}

var _ AKSClusterClient = (*fakeAKSClient)(nil)

type fakeAKSClient struct {
	clusterInfo     *ClusterInfo
	meshProfile     *MeshProfile
	upgradeInfo     *MeshUpgradeInfo
	calls           []string
	enableArgs      aksCallArgs
	canaryArgs      aksCallArgs
	completeArgs    aksCallArgs
	allCompleteArgs []aksCallArgs
	getStateErr     error
	getUpgradeErr   error
	enableErr       error
	canaryErr       error
	completeErr     error
}

func (f *fakeAKSClient) GetClusterState(_ context.Context, rg, cluster string) (*ClusterInfo, *MeshProfile, error) {
	f.calls = append(f.calls, "GetClusterState")
	if f.getStateErr != nil {
		return nil, nil, f.getStateErr
	}
	return f.clusterInfo, f.meshProfile, nil
}

func (f *fakeAKSClient) GetMeshUpgradeTargets(_ context.Context, rg, cluster string) (*MeshUpgradeInfo, error) {
	f.calls = append(f.calls, "GetMeshUpgradeTargets")
	if f.getUpgradeErr != nil {
		return nil, f.getUpgradeErr
	}
	return f.upgradeInfo, nil
}

func (f *fakeAKSClient) EnableMesh(_ context.Context, rg, cluster, revision string) error {
	f.calls = append(f.calls, "EnableMesh")
	f.enableArgs = aksCallArgs{ResourceGroup: rg, ClusterName: cluster, Revision: revision}
	return f.enableErr
}

func (f *fakeAKSClient) StartCanaryUpgrade(_ context.Context, rg, cluster, revision string) error {
	f.calls = append(f.calls, "StartCanaryUpgrade")
	f.canaryArgs = aksCallArgs{ResourceGroup: rg, ClusterName: cluster, Revision: revision}
	return f.canaryErr
}

func (f *fakeAKSClient) CompleteCanaryUpgrade(_ context.Context, rg, cluster, revision string) error {
	f.calls = append(f.calls, "CompleteCanaryUpgrade")
	args := aksCallArgs{ResourceGroup: rg, ClusterName: cluster, Revision: revision}
	f.completeArgs = args
	f.allCompleteArgs = append(f.allCompleteArgs, args)
	return f.completeErr
}

func testCtx(t *testing.T) context.Context {
	return logr.NewContext(context.Background(), testr.New(t))
}

type testCluster struct {
	opts UpgradeOptions
	aks  *fakeAKSClient
	kube *KubeClient
	fake *fake.Clientset
}

type testClusterBuilder struct {
	target            string
	provisioningState string
	revisions         []string
	availableUpgrades []string
	upgradeInProgress bool
	tag               string
	stopAfter         StopAfter
	maxOrphanRetries  int
	objects           []runtime.Object
	aksErrors         map[string]error
}

func newTestCluster(target string) *testClusterBuilder {
	return &testClusterBuilder{
		target:            target,
		provisioningState: "Succeeded",
		aksErrors:         make(map[string]error),
	}
}

func (b *testClusterBuilder) withRevisions(revs ...string) *testClusterBuilder {
	b.revisions = revs
	return b
}

func (b *testClusterBuilder) withAvailableUpgrades(upgrades ...string) *testClusterBuilder {
	b.availableUpgrades = upgrades
	return b
}

func (b *testClusterBuilder) withUpgradeInProgress() *testClusterBuilder {
	b.upgradeInProgress = true
	return b
}

func (b *testClusterBuilder) withTag(tag string) *testClusterBuilder {
	b.tag = tag
	return b
}

func (b *testClusterBuilder) withStopAfter(stop StopAfter) *testClusterBuilder {
	b.stopAfter = stop
	return b
}

func (b *testClusterBuilder) withMaxOrphanRetries(n int) *testClusterBuilder {
	b.maxOrphanRetries = n
	return b
}

func (b *testClusterBuilder) withNamespace(name, revLabel string) *testClusterBuilder {
	b.objects = append(b.objects, &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: name, Labels: map[string]string{"istio.io/rev": revLabel}},
	})
	return b
}

func (b *testClusterBuilder) withDeployment(ns, name string, replicas int32) *testClusterBuilder {
	b.objects = append(b.objects, &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
		Spec:       appsv1.DeploymentSpec{Replicas: ptr.To(replicas)},
		Status:     appsv1.DeploymentStatus{UpdatedReplicas: replicas, ReadyReplicas: replicas},
	})
	return b
}

func (b *testClusterBuilder) withPodOnRevision(ns, name, revision string, owners ...metav1.OwnerReference) *testClusterBuilder {
	b.objects = append(b.objects, &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: name, Namespace: ns,
			Annotations:     map[string]string{"sidecar.istio.io/status": fmt.Sprintf(`{"revision":"%s"}`, revision)},
			OwnerReferences: owners,
		},
		Status: corev1.PodStatus{Phase: corev1.PodRunning},
	})
	return b
}

func (b *testClusterBuilder) withReplicaSet(ns, name, deployName string) *testClusterBuilder {
	b.objects = append(b.objects, &appsv1.ReplicaSet{
		ObjectMeta: metav1.ObjectMeta{
			Name: name, Namespace: ns,
			OwnerReferences: []metav1.OwnerReference{{Name: deployName, Kind: "Deployment", APIVersion: "apps/v1", Controller: ptr.To(true)}},
		},
	})
	return b
}

func (b *testClusterBuilder) withRevisionWebhook(revision string) *testClusterBuilder {
	b.objects = append(b.objects, &admissionregistrationv1.MutatingWebhookConfiguration{
		ObjectMeta: metav1.ObjectMeta{Name: fmt.Sprintf("istio-sidecar-injector-%s-aks-istio-system", revision)},
		Webhooks: []admissionregistrationv1.MutatingWebhook{
			{
				Name: "rev.namespace.sidecar-injector.istio.io",
				ClientConfig: admissionregistrationv1.WebhookClientConfig{
					CABundle: []byte(fmt.Sprintf("ca-bundle-%s", revision)),
					Service:  &admissionregistrationv1.ServiceReference{Name: fmt.Sprintf("istiod-%s", revision), Namespace: "aks-istio-system"},
				},
			},
		},
	})
	return b
}

func (b *testClusterBuilder) withTagWebhook(tag, pointsAtRevision string) *testClusterBuilder {
	b.objects = append(b.objects, &admissionregistrationv1.MutatingWebhookConfiguration{
		ObjectMeta: metav1.ObjectMeta{Name: fmt.Sprintf("istio-revision-tag-%s-aks-istio-system", tag)},
		Webhooks: []admissionregistrationv1.MutatingWebhook{
			{
				Name: "rev.namespace.sidecar-injector.istio.io",
				ClientConfig: admissionregistrationv1.WebhookClientConfig{
					CABundle: []byte(fmt.Sprintf("ca-bundle-%s", pointsAtRevision)),
					Service:  &admissionregistrationv1.ServiceReference{Name: fmt.Sprintf("istiod-%s", pointsAtRevision), Namespace: "aks-istio-system"},
				},
			},
		},
	})
	return b
}

func (b *testClusterBuilder) withHealthyGateway() *testClusterBuilder {
	b.objects = append(b.objects,
		&corev1.Service{
			ObjectMeta: metav1.ObjectMeta{Name: "aks-istio-ingressgateway-external", Namespace: "aks-istio-ingress"},
			Spec:       corev1.ServiceSpec{Type: corev1.ServiceTypeLoadBalancer, Selector: map[string]string{"app": "gw"}},
			Status:     corev1.ServiceStatus{LoadBalancer: corev1.LoadBalancerStatus{Ingress: []corev1.LoadBalancerIngress{{IP: "10.0.0.1"}}}},
		},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name: "gw-pod", Namespace: "aks-istio-ingress",
				Labels:      map[string]string{"app": "gw"},
				Annotations: map[string]string{"sidecar.istio.io/status": `{"revision":"gw"}`},
			},
			Status: corev1.PodStatus{
				Phase:      corev1.PodRunning,
				Conditions: []corev1.PodCondition{{Type: corev1.PodReady, Status: corev1.ConditionTrue}},
			},
		},
	)
	return b
}

func (b *testClusterBuilder) withAKSError(operation string, err error) *testClusterBuilder {
	b.aksErrors[operation] = err
	return b
}

func (b *testClusterBuilder) build(t *testing.T) *testCluster {
	t.Helper()

	// Always include system namespaces
	objects := []runtime.Object{
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "aks-istio-system"}},
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "aks-istio-ingress"}},
	}

	// Add healthy istiod deployments for each revision
	for _, rev := range b.revisions {
		objects = append(objects, &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: fmt.Sprintf("istiod-%s", rev), Namespace: "aks-istio-system"},
			Spec:       appsv1.DeploymentSpec{Replicas: ptr.To[int32](2)},
			Status:     appsv1.DeploymentStatus{AvailableReplicas: 2},
		})
	}

	// Install scenario: no revisions installed but target needs istiod after ARM enable
	if len(b.revisions) == 0 && b.target != "" {
		objects = append(objects, &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: fmt.Sprintf("istiod-%s", b.target), Namespace: "aks-istio-system"},
			Spec:       appsv1.DeploymentSpec{Replicas: ptr.To[int32](2)},
			Status:     appsv1.DeploymentStatus{AvailableReplicas: 2},
		})
	}

	// Add user-specified objects
	objects = append(objects, b.objects...)

	fakeClient := fake.NewSimpleClientset(objects...)

	opts := DefaultUpgradeOptions()
	opts.ResourceGroup = "rg-test"
	opts.ClusterName = "cluster-1"
	opts.Versions = b.target
	opts.Tag = b.tag
	opts.StopAfter = b.stopAfter
	if b.maxOrphanRetries > 0 {
		opts.MaxOrphanRetries = b.maxOrphanRetries
	}

	aks := &fakeAKSClient{
		clusterInfo: &ClusterInfo{
			ProvisioningState: b.provisioningState,
		},
		meshProfile: &MeshProfile{Revisions: b.revisions},
		upgradeInfo: &MeshUpgradeInfo{
			AvailableUpgrades: b.availableUpgrades,
			UpgradeInProgress: b.upgradeInProgress,
		},
		enableErr:     b.aksErrors["EnableMesh"],
		canaryErr:     b.aksErrors["StartCanaryUpgrade"],
		completeErr:   b.aksErrors["CompleteCanaryUpgrade"],
		getStateErr:   b.aksErrors["GetClusterState"],
		getUpgradeErr: b.aksErrors["GetMeshUpgradeTargets"],
	}

	return &testCluster{
		opts: opts,
		aks:  aks,
		kube: NewKubeClientFromInterface(fakeClient),
		fake: fakeClient,
	}
}

func trackerAdd(t *testing.T, client *fake.Clientset, obj runtime.Object) {
	t.Helper()
	require.NoError(t, client.Tracker().Add(obj), "failed to add object to fake tracker")
}

func TestRunUpgrade_EmptyVersions(t *testing.T) {
	tc := newTestCluster("").build(t)
	err := RunUpgrade(testCtx(t), tc.opts, tc.aks, tc.kube)
	assert.ErrorContains(t, err, "no versions specified")
}

func TestRunUpgrade_InvalidVersion(t *testing.T) {
	tc := newTestCluster("asm 1 29!!").build(t)
	err := RunUpgrade(testCtx(t), tc.opts, tc.aks, tc.kube)
	assert.ErrorContains(t, err, "invalid target version")
}

func TestRunUpgrade_DryRun(t *testing.T) {
	tc := newTestCluster("asm-1-29").
		withRevisions("asm-1-28").
		withAvailableUpgrades("asm-1-29").
		build(t)
	tc.opts.DryRun = true

	err := RunUpgrade(testCtx(t), tc.opts, tc.aks, tc.kube)
	require.NoError(t, err)
	assert.NotContains(t, tc.aks.calls, "EnableMesh")
	assert.NotContains(t, tc.aks.calls, "StartCanaryUpgrade")
	assert.NotContains(t, tc.aks.calls, "CompleteCanaryUpgrade")
}

func TestRunUpgrade_AlreadyAtTarget(t *testing.T) {
	tc := newTestCluster("asm-1-29").
		withRevisions("asm-1-29").
		withRevisionWebhook("asm-1-29").
		withTag("prod-stable").
		build(t)

	err := RunUpgrade(testCtx(t), tc.opts, tc.aks, tc.kube)
	require.NoError(t, err)
	assert.NotContains(t, tc.aks.calls, "EnableMesh", "should not call EnableMesh")
	assert.NotContains(t, tc.aks.calls, "StartCanaryUpgrade", "should not call StartCanaryUpgrade")

	cm, err := tc.fake.CoreV1().ConfigMaps("aks-istio-system").Get(
		context.Background(), "istio-shared-configmap-asm-1-29", metav1.GetOptions{})
	require.NoError(t, err, "skip path should ensure ConfigMap exists when at target")
	assert.Equal(t, "asm-1-29", cm.Labels["istio.io/rev"])

	tagWH, err := tc.fake.AdmissionregistrationV1().MutatingWebhookConfigurations().Get(
		context.Background(), "istio-revision-tag-prod-stable-aks-istio-system", metav1.GetOptions{})
	require.NoError(t, err, "skip path should ensure tag webhook exists when at target")
	assert.Equal(t, "istiod-asm-1-29", tagWH.Webhooks[0].ClientConfig.Service.Name)
}

func TestRunUpgrade_Install(t *testing.T) {
	tc := newTestCluster("asm-1-29").
		withHealthyGateway().
		build(t)

	err := RunUpgrade(testCtx(t), tc.opts, tc.aks, tc.kube)
	require.NoError(t, err)
	assert.Contains(t, tc.aks.calls, "EnableMesh")
	assert.NotContains(t, tc.aks.calls, "StartCanaryUpgrade")
	assert.Equal(t, "rg-test", tc.aks.enableArgs.ResourceGroup)
	assert.Equal(t, "cluster-1", tc.aks.enableArgs.ClusterName)
	assert.Equal(t, "asm-1-29", tc.aks.enableArgs.Revision)

	cm, err := tc.fake.CoreV1().ConfigMaps("aks-istio-system").Get(
		context.Background(), "istio-shared-configmap-asm-1-29", metav1.GetOptions{})
	require.NoError(t, err, "install path should create ConfigMap")
	assert.Equal(t, "asm-1-29", cm.Labels["istio.io/rev"])
}

func TestRunUpgrade_InstallWithTag(t *testing.T) {
	tc := newTestCluster("asm-1-29").
		withTag("prod-stable").
		withRevisionWebhook("asm-1-29").
		withHealthyGateway().
		build(t)

	err := RunUpgrade(testCtx(t), tc.opts, tc.aks, tc.kube)
	require.NoError(t, err)
	assert.Contains(t, tc.aks.calls, "EnableMesh", "should call EnableMesh")

	tagWH, err := tc.fake.AdmissionregistrationV1().MutatingWebhookConfigurations().Get(
		context.Background(), "istio-revision-tag-prod-stable-aks-istio-system", metav1.GetOptions{})
	require.NoError(t, err, "install with tag should create the tag webhook")
	assert.Equal(t, "istiod-asm-1-29", tagWH.Webhooks[0].ClientConfig.Service.Name)
	assert.NotEmpty(t, tagWH.Webhooks[0].ClientConfig.CABundle)
}

func TestRunUpgrade_ResumeWithTag(t *testing.T) {
	tc := newTestCluster("asm-1-29").
		withRevisions("asm-1-28", "asm-1-29").
		withUpgradeInProgress().
		withTag("prod-stable").
		withNamespace("app-ns", "prod-stable").
		withTagWebhook("prod-stable", "asm-1-28").
		withRevisionWebhook("asm-1-28").
		withRevisionWebhook("asm-1-29").
		withHealthyGateway().
		build(t)

	err := RunUpgrade(testCtx(t), tc.opts, tc.aks, tc.kube)
	require.NoError(t, err)
	assert.NotContains(t, tc.aks.calls, "StartCanaryUpgrade", "should not call StartCanaryUpgrade")
	assert.Contains(t, tc.aks.calls, "CompleteCanaryUpgrade", "should call CompleteCanaryUpgrade")

	// Tag webhook should now point at the target revision
	tagWH, err := tc.fake.AdmissionregistrationV1().MutatingWebhookConfigurations().Get(
		context.Background(), "istio-revision-tag-prod-stable-aks-istio-system", metav1.GetOptions{})
	require.NoError(t, err)
	assert.Equal(t, "istiod-asm-1-29", tagWH.Webhooks[0].ClientConfig.Service.Name,
		"resume should flip tag webhook to target revision")
	assert.NotEmpty(t, tagWH.Webhooks[0].ClientConfig.CABundle,
		"resume should update CA bundle to target revision")

	// Namespace labels should stay as the tag value
	ns, err := tc.fake.CoreV1().Namespaces().Get(context.Background(), "app-ns", metav1.GetOptions{})
	require.NoError(t, err)
	assert.Equal(t, "prod-stable", ns.Labels["istio.io/rev"],
		"tag-based namespace labels should not be changed during resume")
}

func TestRunUpgrade_Upgrade(t *testing.T) {
	tc := newTestCluster("asm-1-29").
		withRevisions("asm-1-28").
		withAvailableUpgrades("asm-1-29").
		withNamespace("app-ns", "asm-1-28").
		withDeployment("app-ns", "web", 1).
		withHealthyGateway().
		build(t)

	// Manually add target istiod (simulates AKS deploying it after StartCanary)
	trackerAdd(t, tc.fake, &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "istiod-asm-1-29", Namespace: "aks-istio-system"},
		Spec:       appsv1.DeploymentSpec{Replicas: ptr.To[int32](2)},
		Status:     appsv1.DeploymentStatus{AvailableReplicas: 2},
	})

	err := RunUpgrade(testCtx(t), tc.opts, tc.aks, tc.kube)
	require.NoError(t, err)
	assert.Contains(t, tc.aks.calls, "StartCanaryUpgrade", "should call StartCanaryUpgrade")
	assert.Contains(t, tc.aks.calls, "CompleteCanaryUpgrade", "should call CompleteCanaryUpgrade")
	assert.Equal(t, "asm-1-29", tc.aks.canaryArgs.Revision)
	assert.Equal(t, "asm-1-29", tc.aks.completeArgs.Revision)
}

func TestRunUpgrade_DirectRevisionUpdatesNamespaceBeforeRestart(t *testing.T) {
	podOwner := metav1.OwnerReference{Name: "web-abc", Kind: "ReplicaSet", APIVersion: "apps/v1", Controller: ptr.To(true)}

	tc := newTestCluster("asm-1-29").
		withRevisions("asm-1-28").
		withAvailableUpgrades("asm-1-29").
		withNamespace("app-ns", "asm-1-28").
		withDeployment("app-ns", "web", 1).
		withReplicaSet("app-ns", "web-abc", "web").
		withPodOnRevision("app-ns", "web-abc-123", "asm-1-28", podOwner).
		withHealthyGateway().
		build(t)

	// Manually add target istiod (simulates AKS deploying it after StartCanary)
	trackerAdd(t, tc.fake, &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "istiod-asm-1-29", Namespace: "aks-istio-system"},
		Spec:       appsv1.DeploymentSpec{Replicas: ptr.To[int32](2)},
		Status:     appsv1.DeploymentStatus{AvailableReplicas: 2},
	})

	observedRestart := false
	tc.fake.PrependReactor("patch", "deployments", func(action k8stesting.Action) (bool, runtime.Object, error) {
		observedRestart = true
		ns, err := tc.fake.Tracker().Get(corev1.SchemeGroupVersion.WithResource("namespaces"), "", "app-ns")
		if err != nil {
			return true, nil, err
		}
		if got := ns.(*corev1.Namespace).Labels["istio.io/rev"]; got != "asm-1-29" {
			return true, nil, fmt.Errorf("namespace label was %s during restart, expected asm-1-29", got)
		}
		pod, err := tc.fake.Tracker().Get(corev1.SchemeGroupVersion.WithResource("pods"), "app-ns", "web-abc-123")
		if err != nil {
			return true, nil, err
		}
		updated := pod.(*corev1.Pod).DeepCopy()
		updated.Annotations["sidecar.istio.io/status"] = `{"revision":"asm-1-29"}`
		if err := tc.fake.Tracker().Update(corev1.SchemeGroupVersion.WithResource("pods"), updated, "app-ns"); err != nil {
			return true, nil, err
		}
		return false, nil, nil
	})

	err := RunUpgrade(testCtx(t), tc.opts, tc.aks, tc.kube)
	require.NoError(t, err)
	assert.True(t, observedRestart, "stale workload should have been restarted during the upgrade")
}

func TestRunUpgrade_EnableMeshError(t *testing.T) {
	tc := newTestCluster("asm-1-29").
		withAKSError("EnableMesh", fmt.Errorf("ARM 500")).
		withHealthyGateway().
		build(t)

	err := RunUpgrade(testCtx(t), tc.opts, tc.aks, tc.kube)
	assert.ErrorContains(t, err, "ARM 500")
}

func TestRunUpgrade_InstallCPVerificationFailure(t *testing.T) {
	tc := newTestCluster("asm-1-29").build(t)
	// Remove the healthy istiod deployment that builder auto-creates
	_ = tc.fake.AppsV1().Deployments("aks-istio-system").Delete(context.Background(), "istiod-asm-1-29", metav1.DeleteOptions{})

	err := RunUpgrade(testCtx(t), tc.opts, tc.aks, tc.kube)
	assert.ErrorIs(t, err, ErrControlPlaneUnhealthy)
	assert.Contains(t, tc.aks.calls, "EnableMesh", "should call EnableMesh")
}

func TestRunUpgrade_GetClusterStateError(t *testing.T) {
	tc := newTestCluster("asm-1-29").
		withAKSError("GetClusterState", fmt.Errorf("ARM throttled")).
		build(t)

	err := RunUpgrade(testCtx(t), tc.opts, tc.aks, tc.kube)
	assert.ErrorContains(t, err, "failed to get cluster state")
	assert.ErrorContains(t, err, "ARM throttled")
}

func TestRunUpgrade_GetUpgradeTargetsError(t *testing.T) {
	tc := newTestCluster("asm-1-29").
		withRevisions("asm-1-28").
		withAKSError("GetMeshUpgradeTargets", fmt.Errorf("network timeout")).
		build(t)

	err := RunUpgrade(testCtx(t), tc.opts, tc.aks, tc.kube)
	assert.ErrorContains(t, err, "failed to get upgrade targets")
	assert.ErrorContains(t, err, "network timeout")
}

func TestRunUpgrade_Resume(t *testing.T) {
	tc := newTestCluster("asm-1-29").
		withRevisions("asm-1-28", "asm-1-29").
		withUpgradeInProgress().
		withNamespace("app-ns", "asm-1-28").
		withHealthyGateway().
		build(t)

	err := RunUpgrade(testCtx(t), tc.opts, tc.aks, tc.kube)
	require.NoError(t, err)
	assert.NotContains(t, tc.aks.calls, "StartCanaryUpgrade", "should not call StartCanaryUpgrade")
	assert.Contains(t, tc.aks.calls, "CompleteCanaryUpgrade", "should call CompleteCanaryUpgrade")

	cm, err := tc.fake.CoreV1().ConfigMaps("aks-istio-system").Get(
		context.Background(), "istio-shared-configmap-asm-1-29", metav1.GetOptions{})
	require.NoError(t, err, "resume path should ensure ConfigMap exists")
	assert.Equal(t, "asm-1-29", cm.Labels["istio.io/rev"])
}

func TestRunUpgrade_TagBasedNamespacesWithoutTagConfig(t *testing.T) {
	tc := newTestCluster("asm-1-29").
		withRevisions("asm-1-28").
		withAvailableUpgrades("asm-1-29").
		withNamespace("app-ns", "prod-stable").
		withHealthyGateway().
		build(t)

	err := RunUpgrade(testCtx(t), tc.opts, tc.aks, tc.kube)
	assert.ErrorContains(t, err, "tag-based injection labels but no tag is configured")
}

func TestRunUpgrade_OrphanRetrySucceeds(t *testing.T) {
	podOwner := metav1.OwnerReference{Name: "web-abc", Kind: "ReplicaSet", APIVersion: "apps/v1", Controller: ptr.To(true)}

	tc := newTestCluster("asm-1-29").
		withRevisions("asm-1-28", "asm-1-29").
		withNamespace("app-ns", "asm-1-28").
		withDeployment("app-ns", "web", 1).
		withReplicaSet("app-ns", "web-abc", "web").
		withPodOnRevision("app-ns", "web-abc-123", "asm-1-28", podOwner).
		withMaxOrphanRetries(3).
		withHealthyGateway().
		build(t)

	// Simulate pod getting new sidecar only after the orphan retry restart (2nd patch),
	// not the initial restart (1st patch), so the orphan check finds stale pods on its
	// first pass and triggers a real retry iteration.
	patchCount := 0
	tc.fake.PrependReactor("patch", "deployments", func(action k8stesting.Action) (bool, runtime.Object, error) {
		patchCount++
		if patchCount >= 2 {
			pod, _ := tc.fake.Tracker().Get(
				corev1.SchemeGroupVersion.WithResource("pods"),
				"app-ns", "web-abc-123",
			)
			if pod != nil {
				updated := pod.(*corev1.Pod).DeepCopy()
				updated.Annotations["sidecar.istio.io/status"] = `{"revision":"asm-1-29"}`
				_ = tc.fake.Tracker().Update(corev1.SchemeGroupVersion.WithResource("pods"), updated, "app-ns")
			}
		}
		return false, nil, nil
	})

	err := RunUpgrade(testCtx(t), tc.opts, tc.aks, tc.kube)
	require.NoError(t, err)
	assert.Contains(t, tc.aks.calls, "CompleteCanaryUpgrade", "should call CompleteCanaryUpgrade")
	assert.GreaterOrEqual(t, patchCount, 2, "orphan retry loop should have triggered a second restart")
}

func TestValidateStopAfter(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    StopAfter
		wantErr bool
	}{
		{name: "canary-start", input: "canary-start", want: StopAfterCanaryStart},
		{name: "orphan-check", input: "orphan-check", want: StopAfterOrphanCheck},
		{name: "invalid value", input: "bogus", wantErr: true},
		{name: "empty is invalid", input: "", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ValidateStopAfter(tt.input)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.want, got, "unexpected StopAfter for %q", tt.name)
			}
		})
	}
}

func TestRunUpgrade_SkipWithMismatchWarning(t *testing.T) {
	// ARM installed asm-1-29 (default) but config targets asm-1-28 --downgrade skip
	tc := newTestCluster("asm-1-28").
		withRevisions("asm-1-29").
		build(t)

	err := RunUpgrade(testCtx(t), tc.opts, tc.aks, tc.kube)
	require.NoError(t, err)
	assert.NotContains(t, tc.aks.calls, "EnableMesh", "should not call EnableMesh")
	assert.NotContains(t, tc.aks.calls, "StartCanaryUpgrade", "should not call StartCanaryUpgrade")
	assert.NotContains(t, tc.aks.calls, "CompleteCanaryUpgrade", "should not call CompleteCanaryUpgrade")
}

func TestRunUpgrade_FreshClusterUpgradesToConfigTarget(t *testing.T) {
	// ARM installed n-1 (asm-1-28) as default, config targets n (asm-1-29)
	// Go code should trigger canary upgrade to reach config target
	tc := newTestCluster("asm-1-29").
		withRevisions("asm-1-28").
		withAvailableUpgrades("asm-1-29").
		withNamespace("app-ns", "asm-1-28").
		withDeployment("app-ns", "web", 1).
		withHealthyGateway().
		build(t)

	// Manually add target istiod (simulates AKS deploying it after StartCanary)
	trackerAdd(t, tc.fake, &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "istiod-asm-1-29", Namespace: "aks-istio-system"},
		Spec:       appsv1.DeploymentSpec{Replicas: ptr.To[int32](2)},
		Status:     appsv1.DeploymentStatus{AvailableReplicas: 2},
	})

	err := RunUpgrade(testCtx(t), tc.opts, tc.aks, tc.kube)
	require.NoError(t, err)
	assert.Contains(t, tc.aks.calls, "StartCanaryUpgrade", "should call StartCanaryUpgrade")
	assert.Contains(t, tc.aks.calls, "CompleteCanaryUpgrade", "should call CompleteCanaryUpgrade")
	assert.Equal(t, "asm-1-29", tc.aks.canaryArgs.Revision)
}

func TestRunUpgrade_StopAfterCanaryStart(t *testing.T) {
	tc := newTestCluster("asm-1-29").
		withRevisions("asm-1-28").
		withAvailableUpgrades("asm-1-29").
		withStopAfter(StopAfterCanaryStart).
		withHealthyGateway().
		build(t)

	err := RunUpgrade(testCtx(t), tc.opts, tc.aks, tc.kube)
	require.NoError(t, err)
	assert.Contains(t, tc.aks.calls, "StartCanaryUpgrade", "should start canary before stopping")
	assert.NotContains(t, tc.aks.calls, "CompleteCanaryUpgrade", "should not complete canary when stopping after canary-start")
}

func TestRunUpgrade_StopAfterOrphanCheck(t *testing.T) {
	tc := newTestCluster("asm-1-29").
		withRevisions("asm-1-28").
		withAvailableUpgrades("asm-1-29").
		withStopAfter(StopAfterOrphanCheck).
		withNamespace("app-ns", "asm-1-28").
		withDeployment("app-ns", "web", 1).
		withHealthyGateway().
		build(t)

	// Manually add target istiod (simulates AKS deploying it after StartCanary)
	trackerAdd(t, tc.fake, &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "istiod-asm-1-29", Namespace: "aks-istio-system"},
		Spec:       appsv1.DeploymentSpec{Replicas: ptr.To[int32](2)},
		Status:     appsv1.DeploymentStatus{AvailableReplicas: 2},
	})

	err := RunUpgrade(testCtx(t), tc.opts, tc.aks, tc.kube)
	require.NoError(t, err)
	assert.Contains(t, tc.aks.calls, "StartCanaryUpgrade", "should start canary")
	assert.NotContains(t, tc.aks.calls, "CompleteCanaryUpgrade", "should not complete canary when stopping after orphan-check")
}

func TestRunUpgrade_UpgradeWithTag(t *testing.T) {
	tc := newTestCluster("asm-1-29").
		withRevisions("asm-1-28").
		withAvailableUpgrades("asm-1-29").
		withTag("prod-stable").
		withNamespace("app-ns", "prod-stable").
		withDeployment("app-ns", "web", 1).
		withRevisionWebhook("asm-1-29").
		withHealthyGateway().
		build(t)

	// Manually add target istiod (simulates AKS deploying it after StartCanary)
	trackerAdd(t, tc.fake, &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "istiod-asm-1-29", Namespace: "aks-istio-system"},
		Spec:       appsv1.DeploymentSpec{Replicas: ptr.To[int32](2)},
		Status:     appsv1.DeploymentStatus{AvailableReplicas: 2},
	})

	err := RunUpgrade(testCtx(t), tc.opts, tc.aks, tc.kube)
	require.NoError(t, err)
	assert.Contains(t, tc.aks.calls, "StartCanaryUpgrade", "should call StartCanaryUpgrade")
	assert.Contains(t, tc.aks.calls, "CompleteCanaryUpgrade", "should call CompleteCanaryUpgrade")

	ns, err := tc.fake.CoreV1().Namespaces().Get(context.Background(), "app-ns", metav1.GetOptions{})
	require.NoError(t, err)
	assert.Equal(t, "prod-stable", ns.Labels["istio.io/rev"], "tag-based label should be preserved, not changed to direct revision")
}

func TestRunUpgrade_OrphanRetryExhausted(t *testing.T) {
	podOwner := metav1.OwnerReference{Name: "stuck-rs", Kind: "ReplicaSet", APIVersion: "apps/v1", Controller: ptr.To(true)}

	tc := newTestCluster("asm-1-29").
		withRevisions("asm-1-28", "asm-1-29").
		withNamespace("app-ns", "asm-1-28").
		withDeployment("app-ns", "stuck-deploy", 1).
		withReplicaSet("app-ns", "stuck-rs", "stuck-deploy").
		withPodOnRevision("app-ns", "stuck-pod", "asm-1-28", podOwner).
		withMaxOrphanRetries(1).
		withHealthyGateway().
		build(t)

	err := RunUpgrade(testCtx(t), tc.opts, tc.aks, tc.kube)
	assert.ErrorIs(t, err, ErrRetireRevisionWouldOrphanWorkloads)
	assert.NotContains(t, tc.aks.calls, "CompleteCanaryUpgrade", "should not call CompleteCanaryUpgrade")
}

func TestRunUpgrade_HealthCheckFailsRollsBackWorkloads(t *testing.T) {
	tc := newTestCluster("asm-1-29").
		withRevisions("asm-1-28", "asm-1-29").
		withHealthyGateway().
		build(t)

	// Make asm-1-29 unhealthy (0 available replicas)
	deploy, _ := tc.fake.AppsV1().Deployments("aks-istio-system").Get(context.Background(), "istiod-asm-1-29", metav1.GetOptions{})
	deploy.Status.AvailableReplicas = 0
	_, _ = tc.fake.AppsV1().Deployments("aks-istio-system").Update(context.Background(), deploy, metav1.UpdateOptions{})

	err := RunUpgrade(testCtx(t), tc.opts, tc.aks, tc.kube)
	assert.ErrorIs(t, err, ErrControlPlaneUnhealthy, "should return health check error")
	assert.NotContains(t, tc.aks.calls, "CompleteCanaryUpgrade", "should not complete canary on health failure")
}

func TestRunUpgrade_CleanupAndUpgrade(t *testing.T) {
	tc := newTestCluster("asm-1-30").
		withRevisions("asm-1-28", "asm-1-29").
		withAvailableUpgrades("asm-1-30").
		build(t)

	// Manually add asm-1-30 istiod (builder only creates for installed revisions)
	trackerAdd(t, tc.fake, &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "istiod-asm-1-30", Namespace: "aks-istio-system"},
		Spec:       appsv1.DeploymentSpec{Replicas: ptr.To[int32](2)},
		Status:     appsv1.DeploymentStatus{AvailableReplicas: 2},
	})

	// Add gateway with asm-1-28 sidecar
	trackerAdd(t, tc.fake, &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "aks-istio-ingressgateway-external", Namespace: "aks-istio-ingress"},
		Spec:       corev1.ServiceSpec{Type: corev1.ServiceTypeLoadBalancer, Selector: map[string]string{"app": "gw"}},
		Status:     corev1.ServiceStatus{LoadBalancer: corev1.LoadBalancerStatus{Ingress: []corev1.LoadBalancerIngress{{IP: "10.0.0.1"}}}},
	})
	trackerAdd(t, tc.fake, &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: "gw-pod", Namespace: "aks-istio-ingress",
			Labels:      map[string]string{"app": "gw"},
			Annotations: map[string]string{"sidecar.istio.io/status": `{"revision":"asm-1-28"}`},
		},
		Status: corev1.PodStatus{
			Phase:      corev1.PodRunning,
			Conditions: []corev1.PodCondition{{Type: corev1.PodReady, Status: corev1.ConditionTrue}},
		},
	})

	err := RunUpgrade(testCtx(t), tc.opts, tc.aks, tc.kube)
	require.NoError(t, err)

	require.GreaterOrEqual(t, len(tc.aks.allCompleteArgs), 2, "should have two CompleteCanaryUpgrade calls")
	assert.Equal(t, "asm-1-28", tc.aks.allCompleteArgs[0].Revision, "first complete should keep old stable revision")
	assert.Equal(t, "asm-1-30", tc.aks.allCompleteArgs[1].Revision, "second complete should finalize fresh canary")
	assert.Contains(t, tc.aks.calls, "StartCanaryUpgrade", "should call StartCanaryUpgrade")
	assert.Equal(t, "asm-1-30", tc.aks.canaryArgs.Revision, "fresh canary should target new version")
}

func TestRunUpgrade_CleanupAndUpgradeWithTag(t *testing.T) {
	tc := newTestCluster("asm-1-30").
		withRevisions("asm-1-28", "asm-1-29").
		withAvailableUpgrades("asm-1-30").
		withTag("prod-stable").
		withRevisionWebhook("asm-1-28").
		withRevisionWebhook("asm-1-30").
		build(t)

	// Manually add asm-1-30 istiod
	trackerAdd(t, tc.fake, &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "istiod-asm-1-30", Namespace: "aks-istio-system"},
		Spec:       appsv1.DeploymentSpec{Replicas: ptr.To[int32](2)},
		Status:     appsv1.DeploymentStatus{AvailableReplicas: 2},
	})

	// Add gateway with asm-1-28 sidecar
	trackerAdd(t, tc.fake, &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "aks-istio-ingressgateway-external", Namespace: "aks-istio-ingress"},
		Spec:       corev1.ServiceSpec{Type: corev1.ServiceTypeLoadBalancer, Selector: map[string]string{"app": "gw"}},
		Status:     corev1.ServiceStatus{LoadBalancer: corev1.LoadBalancerStatus{Ingress: []corev1.LoadBalancerIngress{{IP: "10.0.0.1"}}}},
	})
	trackerAdd(t, tc.fake, &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: "gw-pod", Namespace: "aks-istio-ingress",
			Labels:      map[string]string{"app": "gw"},
			Annotations: map[string]string{"sidecar.istio.io/status": `{"revision":"asm-1-28"}`},
		},
		Status: corev1.PodStatus{
			Phase:      corev1.PodRunning,
			Conditions: []corev1.PodCondition{{Type: corev1.PodReady, Status: corev1.ConditionTrue}},
		},
	})

	err := RunUpgrade(testCtx(t), tc.opts, tc.aks, tc.kube)
	require.NoError(t, err)
	assert.Contains(t, tc.aks.calls, "CompleteCanaryUpgrade", "should call CompleteCanaryUpgrade")
	assert.Contains(t, tc.aks.calls, "StartCanaryUpgrade", "should call StartCanaryUpgrade")
	require.GreaterOrEqual(t, len(tc.aks.allCompleteArgs), 2)
	assert.Equal(t, "asm-1-28", tc.aks.allCompleteArgs[0].Revision, "cleanup should keep old stable revision")
}

func TestOldRevisionFrom(t *testing.T) {
	tests := []struct {
		name      string
		revisions []string
		target    string
		want      string
	}{
		{
			name:      "single old revision",
			revisions: []string{"asm-1-28", "asm-1-29"},
			target:    "asm-1-29",
			want:      "asm-1-28",
		},
		{
			name:      "multiple old revisions picks highest",
			revisions: []string{"asm-1-27", "asm-1-28", "asm-1-29"},
			target:    "asm-1-29",
			want:      "asm-1-28",
		},
		{
			name:      "only target installed",
			revisions: []string{"asm-1-29"},
			target:    "asm-1-29",
			want:      "",
		},
		{
			name:      "empty revisions",
			revisions: nil,
			target:    "asm-1-29",
			want:      "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := oldRevisionFrom(tt.revisions, tt.target)
			assert.Equal(t, tt.want, got, "unexpected old revision for %q", tt.name)
		})
	}
}

func TestRunUpgrade_RollbackDoubleFailure(t *testing.T) {
	podOwner := metav1.OwnerReference{Name: "web-rs", Kind: "ReplicaSet", APIVersion: "apps/v1", Controller: ptr.To(true)}

	tc := newTestCluster("asm-1-29").
		withRevisions("asm-1-28", "asm-1-29").
		withNamespace("app-ns", "asm-1-28").
		withDeployment("app-ns", "web", 1).
		withReplicaSet("app-ns", "web-rs", "web").
		withPodOnRevision("app-ns", "web-rs-pod", "asm-1-29", podOwner).
		build(t)

	// Make asm-1-29 unhealthy
	deploy, _ := tc.fake.AppsV1().Deployments("aks-istio-system").Get(context.Background(), "istiod-asm-1-29", metav1.GetOptions{})
	deploy.Status.AvailableReplicas = 0
	_, _ = tc.fake.AppsV1().Deployments("aks-istio-system").Update(context.Background(), deploy, metav1.UpdateOptions{})

	// Add gateway with healthy asm-1-29 sidecar
	trackerAdd(t, tc.fake, &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "aks-istio-ingressgateway-external", Namespace: "aks-istio-ingress"},
		Spec:       corev1.ServiceSpec{Type: corev1.ServiceTypeLoadBalancer, Selector: map[string]string{"app": "gw"}},
		Status:     corev1.ServiceStatus{LoadBalancer: corev1.LoadBalancerStatus{Ingress: []corev1.LoadBalancerIngress{{IP: "10.0.0.1"}}}},
	})
	trackerAdd(t, tc.fake, &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: "gw-pod", Namespace: "aks-istio-ingress",
			Labels:      map[string]string{"app": "gw"},
			Annotations: map[string]string{"sidecar.istio.io/status": `{"revision":"asm-1-29"}`},
		},
		Status: corev1.PodStatus{
			Phase:      corev1.PodRunning,
			Conditions: []corev1.PodCondition{{Type: corev1.PodReady, Status: corev1.ConditionTrue}},
		},
	})

	// Make deployment patches fail to simulate rollback failure
	tc.fake.PrependReactor("patch", "deployments", func(action k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, fmt.Errorf("simulated rollback patch failure")
	})

	err := RunUpgrade(testCtx(t), tc.opts, tc.aks, tc.kube)
	assert.ErrorIs(t, err, ErrControlPlaneUnhealthy, "should contain original health check error")
	assert.ErrorContains(t, err, "rollback also failed", "should contain rollback failure")
}

func TestRunUpgrade_StartCanaryError(t *testing.T) {
	tc := newTestCluster("asm-1-29").
		withRevisions("asm-1-28").
		withAvailableUpgrades("asm-1-29").
		withAKSError("StartCanaryUpgrade", fmt.Errorf("ARM 409 conflict")).
		withHealthyGateway().
		build(t)

	err := RunUpgrade(testCtx(t), tc.opts, tc.aks, tc.kube)
	assert.ErrorContains(t, err, "ARM 409 conflict")
	assert.ErrorContains(t, err, "failed to start canary")
	assert.NotContains(t, tc.aks.calls, "CompleteCanaryUpgrade", "should not call CompleteCanaryUpgrade")
}

func TestRunUpgrade_CompleteCanaryFailureMidCleanup(t *testing.T) {
	tc := newTestCluster("asm-1-30").
		withRevisions("asm-1-28", "asm-1-29").
		withAvailableUpgrades("asm-1-30").
		withAKSError("CompleteCanaryUpgrade", fmt.Errorf("ARM timeout on complete")).
		build(t)

	// Add gateway with asm-1-28 sidecar
	trackerAdd(t, tc.fake, &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "aks-istio-ingressgateway-external", Namespace: "aks-istio-ingress"},
		Spec:       corev1.ServiceSpec{Type: corev1.ServiceTypeLoadBalancer, Selector: map[string]string{"app": "gw"}},
		Status:     corev1.ServiceStatus{LoadBalancer: corev1.LoadBalancerStatus{Ingress: []corev1.LoadBalancerIngress{{IP: "10.0.0.1"}}}},
	})
	trackerAdd(t, tc.fake, &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: "gw-pod", Namespace: "aks-istio-ingress",
			Labels:      map[string]string{"app": "gw"},
			Annotations: map[string]string{"sidecar.istio.io/status": `{"revision":"asm-1-28"}`},
		},
		Status: corev1.PodStatus{
			Phase:      corev1.PodRunning,
			Conditions: []corev1.PodCondition{{Type: corev1.PodReady, Status: corev1.ConditionTrue}},
		},
	})

	err := RunUpgrade(testCtx(t), tc.opts, tc.aks, tc.kube)
	assert.ErrorContains(t, err, "cleanup ARM completion failed")
	assert.ErrorContains(t, err, "ARM timeout on complete")
}

func TestRunUpgrade_PostCompleteVerificationFailure(t *testing.T) {
	tc := newTestCluster("asm-1-29").
		withRevisions("asm-1-28", "asm-1-29").
		withHealthyGateway().
		build(t)

	// After CompleteCanaryUpgrade, the target ConfigMap is deleted by
	// external interference. VerifyUpgrade catches the missing ConfigMap.
	configMapDeleted := false
	// We can't easily hook the fake AKS client's CompleteCanaryUpgrade,
	// so instead we use a reactor that deletes the ConfigMap on the
	// DeleteRevisionConfigMap call for the old revision --which runs
	// right after complete. The reactor also deletes the target ConfigMap.
	tc.fake.PrependReactor("delete", "configmaps", func(action k8stesting.Action) (bool, runtime.Object, error) {
		if !configMapDeleted {
			configMapDeleted = true
			// Also delete the target ConfigMap to trigger verification failure
			_ = tc.fake.Tracker().Delete(
				corev1.SchemeGroupVersion.WithResource("configmaps"),
				"aks-istio-system", "istio-shared-configmap-asm-1-29")
		}
		return false, nil, nil
	})

	err := RunUpgrade(testCtx(t), tc.opts, tc.aks, tc.kube)

	assert.Contains(t, tc.aks.calls, "CompleteCanaryUpgrade",
		"canary should have been completed --this state is non-recoverable")
	assert.Error(t, err)
	assert.ErrorContains(t, err, "post-upgrade verification failed")
	assert.NotContains(t, tc.aks.calls, "EnableMesh",
		"should not attempt re-install after failed post-complete verification")
}

func TestRunUpgrade_OverallTimeout(t *testing.T) {
	tc := newTestCluster("asm-1-29").
		withRevisions("asm-1-28").
		withAvailableUpgrades("asm-1-29").
		withAKSError("StartCanaryUpgrade", context.DeadlineExceeded).
		withHealthyGateway().
		build(t)

	tc.opts.OverallTimeout = 1 * time.Nanosecond

	err := RunUpgrade(testCtx(t), tc.opts, tc.aks, tc.kube)
	assert.Error(t, err, "should fail with deadline or canary error")
}

func TestRunUpgrade_HealthCheckFailsVerifiesRollbackRestarted(t *testing.T) {
	podOwner := metav1.OwnerReference{Name: "web-rs", Kind: "ReplicaSet", APIVersion: "apps/v1", Controller: ptr.To(true)}

	tc := newTestCluster("asm-1-29").
		withRevisions("asm-1-28", "asm-1-29").
		withNamespace("app-ns", "asm-1-28").
		withDeployment("app-ns", "web", 1).
		withReplicaSet("app-ns", "web-rs", "web").
		withPodOnRevision("app-ns", "web-rs-pod", "asm-1-29", podOwner).
		build(t)

	// Make asm-1-29 unhealthy
	deploy, _ := tc.fake.AppsV1().Deployments("aks-istio-system").Get(context.Background(), "istiod-asm-1-29", metav1.GetOptions{})
	deploy.Status.AvailableReplicas = 0
	_, _ = tc.fake.AppsV1().Deployments("aks-istio-system").Update(context.Background(), deploy, metav1.UpdateOptions{})

	// Add gateway with healthy asm-1-29 sidecar
	trackerAdd(t, tc.fake, &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "aks-istio-ingressgateway-external", Namespace: "aks-istio-ingress"},
		Spec:       corev1.ServiceSpec{Type: corev1.ServiceTypeLoadBalancer, Selector: map[string]string{"app": "gw"}},
		Status:     corev1.ServiceStatus{LoadBalancer: corev1.LoadBalancerStatus{Ingress: []corev1.LoadBalancerIngress{{IP: "10.0.0.1"}}}},
	})
	trackerAdd(t, tc.fake, &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: "gw-pod", Namespace: "aks-istio-ingress",
			Labels:      map[string]string{"app": "gw"},
			Annotations: map[string]string{"sidecar.istio.io/status": `{"revision":"asm-1-29"}`},
		},
		Status: corev1.PodStatus{
			Phase:      corev1.PodRunning,
			Conditions: []corev1.PodCondition{{Type: corev1.PodReady, Status: corev1.ConditionTrue}},
		},
	})

	rollbackPatchCount := 0
	tc.fake.PrependReactor("patch", "deployments", func(action k8stesting.Action) (bool, runtime.Object, error) {
		rollbackPatchCount++
		return false, nil, nil
	})

	err := RunUpgrade(testCtx(t), tc.opts, tc.aks, tc.kube)
	assert.ErrorIs(t, err, ErrControlPlaneUnhealthy)
	assert.Greater(t, rollbackPatchCount, 0, "rollback should have triggered deployment patches to restore old sidecar")
}

func TestRunUpgrade_HealthCheckFailsRollsBackTagWebhook(t *testing.T) {
	tc := newTestCluster("asm-1-29").
		withRevisions("asm-1-28", "asm-1-29").
		withTag("prod-stable").
		withNamespace("app-ns", "prod-stable").
		withTagWebhook("prod-stable", "asm-1-29").
		withRevisionWebhook("asm-1-28").
		withRevisionWebhook("asm-1-29").
		build(t)

	// Make asm-1-29 unhealthy
	deploy, _ := tc.fake.AppsV1().Deployments("aks-istio-system").Get(context.Background(), "istiod-asm-1-29", metav1.GetOptions{})
	deploy.Status.AvailableReplicas = 0
	_, _ = tc.fake.AppsV1().Deployments("aks-istio-system").Update(context.Background(), deploy, metav1.UpdateOptions{})

	// Add gateway with healthy asm-1-29 sidecar
	trackerAdd(t, tc.fake, &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "aks-istio-ingressgateway-external", Namespace: "aks-istio-ingress"},
		Spec:       corev1.ServiceSpec{Type: corev1.ServiceTypeLoadBalancer, Selector: map[string]string{"app": "gw"}},
		Status:     corev1.ServiceStatus{LoadBalancer: corev1.LoadBalancerStatus{Ingress: []corev1.LoadBalancerIngress{{IP: "10.0.0.1"}}}},
	})
	trackerAdd(t, tc.fake, &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: "gw-pod", Namespace: "aks-istio-ingress",
			Labels:      map[string]string{"app": "gw"},
			Annotations: map[string]string{"sidecar.istio.io/status": `{"revision":"asm-1-29"}`},
		},
		Status: corev1.PodStatus{
			Phase:      corev1.PodRunning,
			Conditions: []corev1.PodCondition{{Type: corev1.PodReady, Status: corev1.ConditionTrue}},
		},
	})

	err := RunUpgrade(testCtx(t), tc.opts, tc.aks, tc.kube)
	assert.ErrorIs(t, err, ErrControlPlaneUnhealthy, "should return health check error")
	assert.NotContains(t, tc.aks.calls, "CompleteCanaryUpgrade", "should not complete canary on health failure")

	tagWH, err := tc.fake.AdmissionregistrationV1().MutatingWebhookConfigurations().Get(
		context.Background(), "istio-revision-tag-prod-stable-aks-istio-system", metav1.GetOptions{})
	require.NoError(t, err)
	assert.Equal(t, "istiod-asm-1-28", tagWH.Webhooks[0].ClientConfig.Service.Name,
		"rollback should flip tag webhook back to old revision")
	assert.NotEmpty(t, tagWH.Webhooks[0].ClientConfig.CABundle,
		"rollback should update CA bundle to old revision")
}

func TestRunUpgrade_OrphanRetryExhaustedRollsBackTagWebhook(t *testing.T) {
	podOwner := metav1.OwnerReference{Name: "stuck-rs", Kind: "ReplicaSet", APIVersion: "apps/v1", Controller: ptr.To(true)}

	tc := newTestCluster("asm-1-29").
		withRevisions("asm-1-28", "asm-1-29").
		withTag("prod-stable").
		withNamespace("app-ns", "prod-stable").
		withDeployment("app-ns", "stuck-deploy", 1).
		withReplicaSet("app-ns", "stuck-rs", "stuck-deploy").
		withPodOnRevision("app-ns", "stuck-pod", "asm-1-28", podOwner).
		withTagWebhook("prod-stable", "asm-1-29").
		withRevisionWebhook("asm-1-28").
		withRevisionWebhook("asm-1-29").
		withMaxOrphanRetries(1).
		withHealthyGateway().
		build(t)

	err := RunUpgrade(testCtx(t), tc.opts, tc.aks, tc.kube)
	assert.ErrorIs(t, err, ErrRetireRevisionWouldOrphanWorkloads)
	assert.NotContains(t, tc.aks.calls, "CompleteCanaryUpgrade", "should not call CompleteCanaryUpgrade")

	tagWH, err := tc.fake.AdmissionregistrationV1().MutatingWebhookConfigurations().Get(
		context.Background(), "istio-revision-tag-prod-stable-aks-istio-system", metav1.GetOptions{})
	require.NoError(t, err)
	assert.Equal(t, "istiod-asm-1-28", tagWH.Webhooks[0].ClientConfig.Service.Name,
		"rollback should flip tag webhook back to old revision")
	assert.NotEmpty(t, tagWH.Webhooks[0].ClientConfig.CABundle,
		"rollback should update CA bundle to old revision")
}

func TestRunUpgrade_CleanupUpdatesNamespaceBeforeRestart(t *testing.T) {
	podOwner := metav1.OwnerReference{Name: "web-abc", Kind: "ReplicaSet", APIVersion: "apps/v1", Controller: ptr.To(true)}

	tc := newTestCluster("asm-1-30").
		withRevisions("asm-1-28", "asm-1-29").
		withAvailableUpgrades("asm-1-30").
		withNamespace("app-ns", "asm-1-29").
		withDeployment("app-ns", "web", 1).
		withReplicaSet("app-ns", "web-abc", "web").
		withPodOnRevision("app-ns", "web-abc-123", "asm-1-29", podOwner).
		build(t)

	// Manually add asm-1-30 istiod
	trackerAdd(t, tc.fake, &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "istiod-asm-1-30", Namespace: "aks-istio-system"},
		Spec:       appsv1.DeploymentSpec{Replicas: ptr.To[int32](2)},
		Status:     appsv1.DeploymentStatus{AvailableReplicas: 2},
	})

	// Add gateway with asm-1-28 sidecar
	trackerAdd(t, tc.fake, &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "aks-istio-ingressgateway-external", Namespace: "aks-istio-ingress"},
		Spec:       corev1.ServiceSpec{Type: corev1.ServiceTypeLoadBalancer, Selector: map[string]string{"app": "gw"}},
		Status:     corev1.ServiceStatus{LoadBalancer: corev1.LoadBalancerStatus{Ingress: []corev1.LoadBalancerIngress{{IP: "10.0.0.1"}}}},
	})
	trackerAdd(t, tc.fake, &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: "gw-pod", Namespace: "aks-istio-ingress",
			Labels:      map[string]string{"app": "gw"},
			Annotations: map[string]string{"sidecar.istio.io/status": `{"revision":"asm-1-28"}`},
		},
		Status: corev1.PodStatus{
			Phase:      corev1.PodRunning,
			Conditions: []corev1.PodCondition{{Type: corev1.PodReady, Status: corev1.ConditionTrue}},
		},
	})

	cleanupRestartVerified := false
	tc.fake.PrependReactor("patch", "deployments", func(action k8stesting.Action) (bool, runtime.Object, error) {
		patchAction := action.(k8stesting.PatchAction)
		if patchAction.GetNamespace() != "app-ns" {
			return false, nil, nil
		}

		ns, err := tc.fake.Tracker().Get(corev1.SchemeGroupVersion.WithResource("namespaces"), "", "app-ns")
		if err != nil {
			return true, nil, err
		}
		nsLabel := ns.(*corev1.Namespace).Labels["istio.io/rev"]

		pod, err := tc.fake.Tracker().Get(corev1.SchemeGroupVersion.WithResource("pods"), "app-ns", "web-abc-123")
		if err != nil {
			return true, nil, err
		}

		if !cleanupRestartVerified {
			if nsLabel != "asm-1-28" {
				return true, nil, fmt.Errorf("namespace label was %s during cleanup restart, expected asm-1-28", nsLabel)
			}
			cleanupRestartVerified = true
		}

		updated := pod.(*corev1.Pod).DeepCopy()
		updated.Annotations["sidecar.istio.io/status"] = fmt.Sprintf(`{"revision":"%s"}`, nsLabel)
		if err := tc.fake.Tracker().Update(corev1.SchemeGroupVersion.WithResource("pods"), updated, "app-ns"); err != nil {
			return true, nil, err
		}
		return false, nil, nil
	})

	err := RunUpgrade(testCtx(t), tc.opts, tc.aks, tc.kube)
	require.NoError(t, err)
	assert.True(t, cleanupRestartVerified, "cleanup phase should have restarted workloads with labels already updated")
}

func TestRunUpgrade_DirectRevisionRollbackUpdatesLabels(t *testing.T) {
	tc := newTestCluster("asm-1-29").
		withRevisions("asm-1-28", "asm-1-29").
		withNamespace("app-ns", "asm-1-28").
		build(t)

	// Make asm-1-29 unhealthy
	deploy, _ := tc.fake.AppsV1().Deployments("aks-istio-system").Get(context.Background(), "istiod-asm-1-29", metav1.GetOptions{})
	deploy.Status.AvailableReplicas = 0
	_, _ = tc.fake.AppsV1().Deployments("aks-istio-system").Update(context.Background(), deploy, metav1.UpdateOptions{})

	// Add gateway with healthy asm-1-29 sidecar
	trackerAdd(t, tc.fake, &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "aks-istio-ingressgateway-external", Namespace: "aks-istio-ingress"},
		Spec:       corev1.ServiceSpec{Type: corev1.ServiceTypeLoadBalancer, Selector: map[string]string{"app": "gw"}},
		Status:     corev1.ServiceStatus{LoadBalancer: corev1.LoadBalancerStatus{Ingress: []corev1.LoadBalancerIngress{{IP: "10.0.0.1"}}}},
	})
	trackerAdd(t, tc.fake, &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: "gw-pod", Namespace: "aks-istio-ingress",
			Labels:      map[string]string{"app": "gw"},
			Annotations: map[string]string{"sidecar.istio.io/status": `{"revision":"asm-1-29"}`},
		},
		Status: corev1.PodStatus{
			Phase:      corev1.PodRunning,
			Conditions: []corev1.PodCondition{{Type: corev1.PodReady, Status: corev1.ConditionTrue}},
		},
	})

	err := RunUpgrade(testCtx(t), tc.opts, tc.aks, tc.kube)
	assert.ErrorIs(t, err, ErrControlPlaneUnhealthy, "should fail due to unhealthy CP")

	ns, err := tc.fake.CoreV1().Namespaces().Get(context.Background(), "app-ns", metav1.GetOptions{})
	require.NoError(t, err)
	assert.Equal(t, "asm-1-28", ns.Labels["istio.io/rev"],
		"rollback should revert namespace label to old revision")
}

func TestEnsureIngress_PartialConfigErrors(t *testing.T) {
	client := NewKubeClientFromInterface(fake.NewSimpleClientset())
	ctx := logr.NewContext(context.Background(), testr.New(t))

	tests := []struct {
		name          string
		ingressIPName string
		regionRG      string
		wantErr       bool
	}{
		{
			name:          "both empty is no-op",
			ingressIPName: "",
			regionRG:      "",
			wantErr:       false,
		},
		{
			name:          "only IngressIPName set errors",
			ingressIPName: "my-ip",
			regionRG:      "",
			wantErr:       true,
		},
		{
			name:          "only RegionRG set errors",
			ingressIPName: "",
			regionRG:      "my-rg",
			wantErr:       true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			opts := UpgradeOptions{
				IngressIPName: tc.ingressIPName,
				RegionRG:      tc.regionRG,
			}
			err := ensureIngress(ctx, client, opts)
			if tc.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "incomplete")
			} else {
				require.NoError(t, err)
			}
		})
	}
}
