package helm

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-logr/logr/testr"
	helmreleasev1 "helm.sh/helm/v4/pkg/release/v1"

	kapierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/util/retry"
)

func TestRemoveOldFieldManager(t *testing.T) {
	for _, tc := range []struct {
		name           string
		legacy         bool
		conflict       bool
		alwaysConflict bool
		forbidden      bool
		missing        bool
		disappear      bool
	}{
		{name: "no legacy manager needs no write"},
		{name: "removes only legacy ownership", legacy: true},
		{name: "refetches after concurrent status update", legacy: true, conflict: true},
		{name: "bounds persistent conflicts", legacy: true, alwaysConflict: true},
		{name: "does not retry forbidden updates", legacy: true, forbidden: true},
		{name: "ignores missing resources", missing: true},
		{name: "ignores resources deleted during conflict retry", legacy: true, conflict: true, disappear: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			current := &unstructured.Unstructured{Object: map[string]interface{}{
				"apiVersion": "apps/v1", "kind": "DaemonSet",
				"metadata": map[string]interface{}{"name": "arobit-forwarder", "namespace": "arobit", "resourceVersion": "1"},
				"spec":     map[string]interface{}{"minReadySeconds": int64(5)},
				"status":   map[string]interface{}{"numberReady": int64(12)},
			}}
			fields := []metav1.ManagedFieldsEntry{{Manager: "helmdeploy", Operation: metav1.ManagedFieldsOperationApply, APIVersion: "apps/v1"}}
			if tc.legacy {
				fields = append(fields, metav1.ManagedFieldsEntry{Manager: "helm", Operation: metav1.ManagedFieldsOperationUpdate, APIVersion: "apps/v1"})
			}
			current.SetManagedFields(fields)
			gets, updates := 0, 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				fail := func(code int, reason metav1.StatusReason) {
					w.WriteHeader(code)
					_ = json.NewEncoder(w).Encode(&metav1.Status{TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "Status"}, Status: metav1.StatusFailure, Code: int32(code), Reason: reason, Message: string(reason)})
				}
				if r.URL.Path != "/apis/apps/v1/namespaces/arobit/daemonsets/arobit-forwarder" {
					fail(http.StatusNotFound, metav1.StatusReasonNotFound)
					return
				}
				switch r.Method {
				case http.MethodGet:
					gets++
					if tc.missing || (tc.disappear && updates > 0) {
						fail(http.StatusNotFound, metav1.StatusReasonNotFound)
						return
					}
					_ = json.NewEncoder(w).Encode(current)
				case http.MethodPut:
					updates++
					if tc.forbidden {
						fail(http.StatusForbidden, metav1.StatusReasonForbidden)
						return
					}
					var updated unstructured.Unstructured
					if err := json.NewDecoder(r.Body).Decode(&updated); err != nil {
						fail(http.StatusBadRequest, metav1.StatusReasonBadRequest)
						return
					}
					if (tc.conflict && updates == 1) || tc.alwaysConflict {
						current.SetResourceVersion(fmt.Sprint(updates + 1))
						current.SetAnnotations(map[string]string{"concurrent": "preserved"})
						_ = unstructured.SetNestedField(current.Object, int64(13), "status", "numberReady")
						current.SetManagedFields(append(current.GetManagedFields(), metav1.ManagedFieldsEntry{Manager: "kube-controller-manager", Subresource: "status"}))
					}
					if updated.GetResourceVersion() != current.GetResourceVersion() {
						fail(http.StatusConflict, metav1.StatusReasonConflict)
						return
					}
					current = &updated
					_ = json.NewEncoder(w).Encode(current)
				default:
					fail(http.StatusMethodNotAllowed, metav1.StatusReasonMethodNotAllowed)
				}
			}))
			defer server.Close()
			client, err := dynamic.NewForConfig(&rest.Config{Host: server.URL})
			if err != nil {
				t.Fatal(err)
			}
			mapper := meta.NewDefaultRESTMapper([]schema.GroupVersion{{Group: "apps", Version: "v1"}})
			mapper.Add(schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "DaemonSet"}, meta.RESTScopeNamespace)
			err = removeOldFieldManager(context.Background(), testr.New(t), &Options{completedOptions: &completedOptions{DynamicClient: client, RESTMapper: mapper, ReleaseNamespace: "arobit"}}, &helmreleasev1.Release{Manifest: "apiVersion: apps/v1\nkind: DaemonSet\nmetadata:\n  name: arobit-forwarder\n"})
			switch {
			case tc.alwaysConflict:
				if !kapierrors.IsConflict(err) || updates != retry.DefaultRetry.Steps {
					t.Fatalf("expected bounded conflict retries, got updates=%d err=%v", updates, err)
				}
			case tc.forbidden:
				if !kapierrors.IsForbidden(err) || updates != 1 {
					t.Fatalf("expected immediate forbidden error, got updates=%d err=%v", updates, err)
				}
			default:
				if err != nil {
					t.Fatal(err)
				}
				if tc.missing || !tc.legacy {
					if updates != 0 {
						t.Fatalf("cleanup must not write absent legacy ownership, got %d updates", updates)
					}
					return
				}
				if tc.disappear {
					return
				}
				for _, field := range current.GetManagedFields() {
					if field.Manager == "helm" {
						t.Fatal("legacy ownership remains")
					}
				}
				wantManagers := 1
				if tc.conflict {
					wantManagers = 2
					ready, _, _ := unstructured.NestedInt64(current.Object, "status", "numberReady")
					if gets != 2 || updates != 2 || ready != 13 || current.GetAnnotations()["concurrent"] != "preserved" {
						t.Fatalf("concurrent state lost: gets=%d updates=%d object=%v", gets, updates, current.Object)
					}
				}
				if len(current.GetManagedFields()) != wantManagers || current.GetManagedFields()[0].Manager != "helmdeploy" {
					t.Fatalf("unrelated ownership lost: %v", current.GetManagedFields())
				}
				if value, _, _ := unstructured.NestedInt64(current.Object, "spec", "minReadySeconds"); value != 5 {
					t.Fatal("cleanup modified spec")
				}
			}
		})
	}
}
