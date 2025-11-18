package helm

import (
	"bytes"
	"io"
	"os"
	"testing"

	"github.com/go-logr/logr/testr"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	yamlutil "k8s.io/apimachinery/pkg/util/yaml"
	"sigs.k8s.io/yaml"
)

func TestProcessObject_Pod(t *testing.T) {
	// Create a Pod object
	podObj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Pod",
			"metadata": map[string]interface{}{
				"name":      "test-pod",
				"namespace": "test-namespace",
			},
			"status": map[string]interface{}{
				"phase": "Running",
				"containerStatuses": []interface{}{
					map[string]interface{}{
						"name":  "test-container",
						"ready": true,
						"state": map[string]interface{}{
							"running": map[string]interface{}{
								"startedAt": "2025-11-17T00:00:00Z",
							},
						},
					},
				},
			},
		},
	}

	owner, resource, pod, err := processObject(podObj)
	if err != nil {
		t.Fatalf("processObject failed: %v", err)
	}

	// Should return pod, but not owner or resource
	if owner != nil {
		t.Error("Did not expect owner reference for Pod")
	}
	if resource != nil {
		t.Error("Did not expect resource info for Pod")
	}
	if pod == nil {
		t.Fatal("Expected pod info for Pod")
	}

	// Verify pod details
	if pod.Name != "test-pod" {
		t.Errorf("Expected pod name test-pod, got %s", pod.Name)
	}
	if pod.Namespace != "test-namespace" {
		t.Errorf("Expected pod namespace test-namespace, got %s", pod.Namespace)
	}
	if pod.Phase != "Running" {
		t.Errorf("Expected pod phase Running, got %s", pod.Phase)
	}
}

func TestProcessObject_Deployment(t *testing.T) {
	// Create a Deployment object
	deployment := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "apps/v1",
			"kind":       "Deployment",
			"metadata": map[string]interface{}{
				"name":      "test-deployment",
				"namespace": "test-namespace",
			},
		},
	}

	owner, resource, pod, err := processObject(deployment)
	if err != nil {
		t.Fatalf("processObject failed: %v", err)
	}

	// Should return owner and resource, but not pod
	if owner == nil {
		t.Error("Expected owner reference for Deployment")
	}
	if resource == nil {
		t.Error("Expected resource info for Deployment")
	}
	if pod != nil {
		t.Error("Did not expect pod info for Deployment")
	}

	// Verify owner details
	if owner != nil {
		if owner.Kind != "Deployment" {
			t.Errorf("Expected owner kind Deployment, got %s", owner.Kind)
		}
		if owner.Name != "test-deployment" {
			t.Errorf("Expected owner name test-deployment, got %s", owner.Name)
		}
		if owner.Namespace != "test-namespace" {
			t.Errorf("Expected owner namespace test-namespace, got %s", owner.Namespace)
		}
	}

	// Verify resource details
	if resource != nil {
		if resource.Kind != "Deployment" {
			t.Errorf("Expected resource kind Deployment, got %s", resource.Kind)
		}
		if resource.Name != "test-deployment" {
			t.Errorf("Expected resource name test-deployment, got %s", resource.Name)
		}
		if resource.Namespace != "test-namespace" {
			t.Errorf("Expected resource namespace test-namespace, got %s", resource.Namespace)
		}
	}
}

func TestProcessObject_Service(t *testing.T) {
	// Create a Service object (not a workload owner)
	service := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Service",
			"metadata": map[string]interface{}{
				"name":      "test-service",
				"namespace": "test-namespace",
			},
		},
	}

	owner, resource, pod, err := processObject(service)
	if err != nil {
		t.Fatalf("processObject failed: %v", err)
	}

	// Should return only resource (Service is not a workload owner)
	if owner != nil {
		t.Error("Did not expect owner reference for Service")
	}
	if resource == nil {
		t.Error("Expected resource info for Service")
	}
	if pod != nil {
		t.Error("Did not expect pod info for Service")
	}

	// Verify resource details
	if resource != nil {
		if resource.Kind != "Service" {
			t.Errorf("Expected resource kind Service, got %s", resource.Kind)
		}
		if resource.Name != "test-service" {
			t.Errorf("Expected resource name test-service, got %s", resource.Name)
		}
	}
}

func TestEvaluateResources(t *testing.T) {
	tests := []string{
		"testdata/resources_info_example.yaml",
		"testdata/resources_two_deployments.yaml",
	}

	for _, filename := range tests {
		t.Run(filename, evaluateResourcesHelper(filename))
	}
}

func evaluateResourcesHelper(filename string) func(*testing.T) {
return func(t *testing.T) {
	
	logger := testr.New(t)

	// Load test data from manifest file
	manifestBytes, err := os.ReadFile(filename)
	if err != nil {
		t.Fatalf("Failed to read %s: %v", filename, err)
	}

	// Parse the manifest using the same approach as Helm does
	inputDecoder := yamlutil.NewYAMLOrJSONDecoder(bytes.NewBuffer(manifestBytes), 4096)
	var runtimeObjects []runtime.Object
	
	for {
		ext := runtime.RawExtension{}
		if err := inputDecoder.Decode(&ext); err != nil {
			if err == io.EOF {
				break
			}
			t.Fatalf("Failed to parse manifest: %v", err)
		}
		
		ext.Raw = bytes.TrimSpace(ext.Raw)
		if len(ext.Raw) == 0 || bytes.Equal(ext.Raw, []byte("null")) {
			continue
		}

		obj := &unstructured.Unstructured{}
		if err := yaml.Unmarshal(ext.Raw, obj); err != nil {
			t.Fatalf("Failed to unmarshal object: %v", err)
		}

		runtimeObjects = append(runtimeObjects, obj)
		t.Logf("Added resource of kind %s: %s", obj.GetKind(), obj.GetName())
	}

	t.Logf("Total runtime objects created: %d", len(runtimeObjects))

	// call evaluateResources
	ownerRefs, resources, foundPods, err := evaluateResources(logger, runtimeObjects)
	if err != nil {
		t.Fatalf("evaluateResources failed: %v", err)
	}

	t.Logf("Found %d owner references", len(ownerRefs))
	t.Logf("Found %d resources", len(resources))
	t.Logf("Found %d pods", len(foundPods))

	// Assertions based on the test file content
	// We expect to find at least one Deployment
	hasDeployment := false
	for _, res := range resources {
		if res.Kind == "Deployment" {
			hasDeployment = true
			t.Logf("Found Deployment: %s in namespace %s", res.Name, res.Namespace)
		}
	}
	if !hasDeployment {
		t.Error("Expected to find at least one Deployment in resources")
	}

	// We expect to find the Deployment as an owner reference
	hasDeploymentOwner := false
	for _, owner := range ownerRefs {
		if owner.Kind == "Deployment" {
			hasDeploymentOwner = true
			t.Logf("Found Deployment owner: %s in namespace %s", owner.Name, owner.Namespace)
		}
	}
	if !hasDeploymentOwner {
		t.Error("Expected to find at least one Deployment in owner references")
	}

	// We expect to find pods
	if len(foundPods) == 0 {
		t.Error("Expected to find pods in the resources")
	}

	for _, pod := range foundPods {
		t.Logf("Found pod: %s in namespace %s, phase: %s, state: %s",
			pod.Name, pod.Namespace, pod.Phase, pod.State)
	}

	for _, pod := range foundPods {
		if pod.Name == "" {
			t.Error("Found pod with empty name")
		}
		if pod.Namespace == "" {
			t.Error("Found pod with empty namespace")
		}
		if pod.Phase == "" {
			t.Error("Found pod with empty phase")
		}
		// State might be empty for some pods, so we just log it
		t.Logf("Pod %s state: %s", pod.Name, pod.State)
	}

	for _, res := range resources {
		if res.Kind == "" {
			t.Error("Found resource with empty kind")
		}
		if res.Name == "" {
			t.Error("Found resource with empty name")
		}
	}
}
}
