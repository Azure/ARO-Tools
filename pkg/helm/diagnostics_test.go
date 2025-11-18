package helm

import (
	"bytes"
	"io"
	"os"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/go-logr/logr/testr"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	yamlutil "k8s.io/apimachinery/pkg/util/yaml"

	// "github.com/kubernetes-sigs/prow"

	"sigs.k8s.io/yaml"
)

func TestProcessObject(t *testing.T) {
	tests := []struct {
		name         string
		inputObject  *unstructured.Unstructured
		wantOwner    *OwnerRefInfo
		wantResource *ResourceInfo
		wantPod      *PodInfo
	}{
		{
			name: "pod with running container",
			inputObject: &unstructured.Unstructured{
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
			},
			wantOwner:    nil,
			wantResource: nil,
			wantPod: &PodInfo{
				Name:      "test-pod",
				Namespace: "test-namespace",
				Phase:     "Running",
				State:     "test-container:Running",
			},
		},
		{
			name: "deployment as owner resource",
			inputObject: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "apps/v1",
					"kind":       "Deployment",
					"metadata": map[string]interface{}{
						"name":      "test-deployment",
						"namespace": "test-namespace",
					},
				},
			},
			wantOwner: &OwnerRefInfo{
				Kind:      "Deployment",
				Name:      "test-deployment",
				Namespace: "test-namespace",
			},
			wantResource: &ResourceInfo{
				Kind:      "Deployment",
				Name:      "test-deployment",
				Namespace: "test-namespace",
			},
			wantPod: nil,
		},
		{
			name: "service as non-owner resource",
			inputObject: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "v1",
					"kind":       "Service",
					"metadata": map[string]interface{}{
						"name":      "test-service",
						"namespace": "test-namespace",
					},
				},
			},
			wantOwner: nil,
			wantResource: &ResourceInfo{
				Kind:      "Service",
				Name:      "test-service",
				Namespace: "test-namespace",
			},
			wantPod: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotOwner, gotResource, gotPod, err := processObject(tt.inputObject)
			if err != nil {
				t.Fatalf("processObject() error = %v", err)
			}

			if diff := cmp.Diff(tt.wantOwner, gotOwner); diff != "" {
				t.Errorf("owner mismatch (-want +got):\n%s", diff)
			}
			if diff := cmp.Diff(tt.wantResource, gotResource); diff != "" {
				t.Errorf("resource mismatch (-want +got):\n%s", diff)
			}
			if diff := cmp.Diff(tt.wantPod, gotPod); diff != "" {
				t.Errorf("pod mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

type TestInfoEvalResources struct {
	name      string
	inputFile string
}

func TestEvaluateResources(t *testing.T) {

	tests := []TestInfoEvalResources{
		{"resources_info_example", "testdata/resources_info_example.yaml"},
		{"resources_two_deployments", "testdata/resources_two_deployments.yaml"},
	}

	for _, test := range tests {
		t.Run(test.name, evaluateResourcesHelper(test.inputFile))
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
