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

	"github.com/Azure/ARO-Tools/internal/testutil"
)

func TestProcessObject(t *testing.T) {
	tests := []struct {
		name        string
		inputObject *unstructured.Unstructured
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
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotOwner, gotResource, gotPod, err := processObject(tt.inputObject)
			if err != nil {
				t.Fatalf("processObject() error = %v", err)
			}

			results := struct {
				Owner    *OwnerRefInfo
				Resource *ResourceInfo
				Pod      *PodInfo
			}{
				Owner:    gotOwner,
				Resource: gotResource,
				Pod:      gotPod,
			}

			testutil.CompareWithFixture(t, results)
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
		}

		// call evaluateResources
		ownerRefs, resources, foundPods, err := evaluateResources(logger, runtimeObjects)
		if err != nil {
			t.Fatalf("evaluateResources failed: %v", err)
		}

		t.Logf("Found %d owner references", len(ownerRefs))
		t.Logf("Found %d resources", len(resources))
		t.Logf("Found %d pods", len(foundPods))

		// Compare results with golden fixture
		results := struct {
			OwnerRefs []OwnerRefInfo
			Resources []ResourceInfo
			Pods      []PodInfo
		}{
			OwnerRefs: ownerRefs,
			Resources: resources,
			Pods:      foundPods,
		}

		testutil.CompareWithFixture(t, results)
	}
}
