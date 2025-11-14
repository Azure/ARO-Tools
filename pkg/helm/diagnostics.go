package helm

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"fmt"
	"slices"
	"strings"

	"github.com/go-logr/logr"
	helmreleasev1 "helm.sh/helm/v4/pkg/release/v1"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
)

type PodInfo struct {
	Name      string
	Namespace string
	Phase     string
	State     string // container state summary
}
type ResourceInfo struct {
	Kind      string
	Name      string
	Namespace string
}

type OwnerRefInfo struct {
	Kind          string
	Name          string
	Namespace     string
	KustoDeepLink string
}

// isKustoConfigured checks if necessary options are set for Kusto diagnostics
func isKustoConfigured(opts *Options) bool {
	return (opts.KustoCluster != "" && opts.KustoDatabase != "" && opts.KustoTable != "")
}

// queryToDeepLink compresses the input text with gzip and then encodes it to base64
// Necessary to compress long queries to fit in the default browser URI length limits
// Returns a kusto deep link with proper cluster and database
// see: https://learn.microsoft.com/en-us/kusto/api/rest/deeplink
func queryToDeepLink(text, kustoCluster, kustoDatabase string) (string, error) {
	var buf bytes.Buffer
	gzipWriter := gzip.NewWriter(&buf)

	if _, err := gzipWriter.Write([]byte(text)); err != nil {
		return "", fmt.Errorf("failed to write to gzip writer: %w", err)
	}

	if err := gzipWriter.Close(); err != nil {
		return "", fmt.Errorf("failed to close gzip writer: %w", err)
	}

	encodedQuery := base64.StdEncoding.EncodeToString(buf.Bytes())
	kustoDeepLink := fmt.Sprintf("https://dataexplorer.azure.com/clusters/%s/databases/%s?query=%s", kustoCluster, kustoDatabase, encodedQuery)
	return kustoDeepLink, nil
}

// processObject processes a single runtime.Object and extracts pod information if applicable
func processObject(resources []ResourceInfo, foundPods []PodInfo, ownerRefs map[string][]OwnerRefInfo, item runtime.Object) ([]ResourceInfo, []PodInfo, map[string][]OwnerRefInfo, error) {

	kind := item.GetObjectKind().GroupVersionKind().Kind

	if kind == "Pod" {

		unstructuredItem, ok := item.(*unstructured.Unstructured)
		if !ok {
			return resources, foundPods, ownerRefs, fmt.Errorf("failed to convert item to unstructured: item is not *unstructured.Unstructured")
		}

		var pod corev1.Pod
		err := runtime.DefaultUnstructuredConverter.FromUnstructured(unstructuredItem.Object, &pod)
		if err != nil {
			return resources, foundPods, ownerRefs, fmt.Errorf("failed to convert unstructured to Pod: %w", err)
		}

		podInfo := PodInfo{
			Name:      pod.Name,
			Namespace: pod.Namespace,
			Phase:     string(pod.Status.Phase),
			State:     extractContainerStateSummary(pod.Status.ContainerStatuses),
		}
		foundPods = append(foundPods, podInfo)
	} else {

		objMeta, err := meta.Accessor(item)
		if err != nil {
			return resources, foundPods, ownerRefs, fmt.Errorf("failed to get metadata for item of kind %s: %w", kind, err)
		}
		// process resources separately from pods
		resourceInfo := ResourceInfo{
			Kind:      kind,
			Name:      objMeta.GetName(),
			Namespace: objMeta.GetNamespace(),
		}
		resources = append(resources, resourceInfo)

		// ownerKinds includes all workload resources that can create pods
		ownerKinds := []string{"ReplicaSet", "Deployment", "StatefulSet", "DaemonSet", "Job", "CronJob"}
		if slices.Contains(ownerKinds, kind) {
			addOwnerNoDuplicates(ownerRefs, OwnerRefInfo{
				Kind:      kind,
				Name:      objMeta.GetName(),
				Namespace: objMeta.GetNamespace(),
			})
		}
	}
	return resources, foundPods, ownerRefs, nil
}

// addOwnerNoDuplicates adds a new owner reference to the map, removing any existing owners
// that have a prefix relationship with the new owner to avoid duplicates.
func addOwnerNoDuplicates(ownerRefs map[string][]OwnerRefInfo, newOwner OwnerRefInfo) {
	namespace := newOwner.Namespace

	// Check for prefix conflicts with existing owners in this namespace
	shouldAdd := true
	for i, ownerRef := range ownerRefs[namespace] {
		if strings.HasPrefix(newOwner.Name, ownerRef.Name) {
			// New owner has existing owner as prefix, skip adding
			shouldAdd = false
			break
		} else if strings.HasPrefix(ownerRef.Name, newOwner.Name) {
			// Existing owner has new owner as prefix, remove existing owner (replace with new owner later)
			ownerRefs[namespace] = append(ownerRefs[namespace][:i], ownerRefs[namespace][i+1:]...)
			break
		}
	}

	// If not found in the map or no prefix conflicts, add the new owner
	if shouldAdd {
		ownerRefs[namespace] = append(ownerRefs[namespace], newOwner)
	}
}

// evaluateResources processes the resources in the Helm release to extract owner references, resource info, and pod info
func evaluateResources(logger logr.Logger, release *helmreleasev1.Release) (map[string][]OwnerRefInfo, []ResourceInfo, []PodInfo, error) {

	ownerRefs := make(map[string][]OwnerRefInfo)
	var resources []ResourceInfo
	var foundPods []PodInfo
	var err error

	for _, resourceList := range release.Info.Resources {
		for _, resource := range resourceList {

			if meta.IsListType(resource) {

				if _, ok := resource.(*unstructured.UnstructuredList); !ok {
					logger.Error(fmt.Errorf("resource is not UnstructuredList"), "Failed to process list resource", "kind", resource.GetObjectKind().GroupVersionKind().Kind)
				}

				items, err := meta.ExtractList(resource)
				if err != nil {
					logger.Error(err, "Failed to extract items from list resource", "kind", resource.GetObjectKind().GroupVersionKind().Kind)
				}
				for _, obj := range items {
					resources, foundPods, ownerRefs, err = processObject(resources, foundPods, ownerRefs, obj)
					if err != nil {
						logger.Error(err, "Failed to process list item", "kind", resource.GetObjectKind().GroupVersionKind().Kind)
					}
				}

			} else {
				resources, foundPods, ownerRefs, err = processObject(resources, foundPods, ownerRefs, resource)
				if err != nil {
					logger.Error(err, "Failed to process resource", "kind", resource.GetObjectKind().GroupVersionKind().Kind)
				}
			}
		}
	}
	return ownerRefs, resources, foundPods, nil
}

// Create kusto deep link for all kube events if configuration available
func getKubeEventsQuery(opts *Options, resources []ResourceInfo, deploymentStart string, deploymentEnd string) (string, error) {

	if len(resources) == 0 {
		return "", fmt.Errorf("no resources found in release to build kube events query")
	}

	if !isKustoConfigured(opts) {
		return "", nil
	}

	// Build resource rows for Kusto query
	var resourceRows []string
	for _, resource := range resources {
		resourceRows = append(resourceRows, fmt.Sprintf(`    "%s", "%s", "%s"`, resource.Kind, resource.Name, resource.Namespace))
	}

	// Build kusto query with datatable for resources found in the Helm release
	// Utilizing ['time'] instead of TIMESTAMP since TIMESTAMP rounds down times to the nearest minute,
	// which can lead to missing logs
	// 'time' and 'kind' are reserved keywords in Kusto and need to be escaped with brackets to reference the column name
	kustoQuery := fmt.Sprintf(`
let resources = datatable(['kind']:string, name:string, namespace:string)[
%s
];
%s
| where ['time'] between (datetime("%s") .. datetime("%s"))
| where pod_name startswith "kube-events"
| extend parsed_log = parse_json(log)
| extend ['kind'] = tostring(parsed_log.involved_object.kind),
name = tostring(parsed_log.involved_object.name),
namespace = tostring(parsed_log.involved_object.namespace)
| join kind=inner resources on ['kind'], name, namespace
| project ['time'], pod_name, ['kind'], name, namespace, log
| order by ['time'] desc`, strings.Join(resourceRows, ",\n"), opts.KustoTable, deploymentStart, deploymentEnd)

	return queryToDeepLink(kustoQuery, opts.KustoCluster, opts.KustoDatabase)
}

func getIndivPodQuery(opts *Options, pod PodInfo, deploymentStart string, deploymentEnd string) (string, error) {
	// Create a kusto link for individual failing pods
	if !isKustoConfigured(opts) {
		return "Kusto configuration not provided, skipping Kusto deep link generation.", nil
	}

	if (pod.Phase != "Running" && pod.Phase != "Succeeded") ||
		strings.Contains(pod.State, "CrashLoopBackOff") ||
		strings.Contains(pod.State, "Error") ||
		strings.Contains(pod.State, "Terminated") {

		podQuery := fmt.Sprintf(`%s
| where ['time'] between (datetime("%s") .. datetime("%s"))
| where pod_name == "%s"
| where namespace_name == "%s"
| project ['time'], log, pod_name
| order by ['time'] asc`, opts.KustoTable, deploymentStart, deploymentEnd, pod.Name, pod.Namespace)

		return queryToDeepLink(podQuery, opts.KustoCluster, opts.KustoDatabase)
	} else {
		return "Pod is healthy, no deep link generated.", nil
	}
}

// Create kusto deep link for failing pods if configuration available
// Output a kusto query for all pods to catch possible race conditions or false negatives
func getPodsQuery(logger logr.Logger, opts *Options, foundPods []PodInfo, deploymentStart string, deploymentEnd string) (map[PodInfo]string, string, error) {
	if len(foundPods) == 0 {
		return nil, "", fmt.Errorf("no pods found in release to log")
	}
	var podConditions []string
	podToQueryMap := make(map[PodInfo]string)

	for i := range foundPods {
		pod := foundPods[i]
		// Add string condition for each pod to find them all in one query later
		podConditions = append(podConditions, fmt.Sprintf(`(pod_name == "%s" and namespace_name == "%s")`, pod.Name, pod.Namespace))

		if podQuery, err := getIndivPodQuery(opts, pod, deploymentStart, deploymentEnd); err != nil {
			logger.Error(err, "Failed to create Kusto deep link for failing pod", "pod", pod.Name, "namespace", pod.Namespace)
		} else if podQuery != "" {
			podToQueryMap[pod] = podQuery
		}
	}

	// Still want to output logs for individual pods, but no kusto query for all pods
	if !isKustoConfigured(opts) {
		return podToQueryMap, "", nil
	}

	allPodsQuery := fmt.Sprintf(`%s
| where ['time'] between (datetime("%s") .. datetime("%s"))
| where %s
| project ['time'], log, pod_name, namespace_name
| order by pod_name asc, ['time'] asc`,
		opts.KustoTable,
		deploymentStart,
		deploymentEnd,
		strings.Join(podConditions, " or "))

	allPodsLink, err := queryToDeepLink(allPodsQuery, opts.KustoCluster, opts.KustoDatabase)
	if err != nil {
		return podToQueryMap, "", err
	}
	return podToQueryMap, allPodsLink, nil
}

// getWorkloadResourcePodsLink creates Kusto deep links for pods managed by workload resources
// (Deployments, ReplicaSets, StatefulSets, etc.) using prefix matching. This serves as a
// safety net to catch any pods that might have been missed during direct pod enumeration,
// particularly useful for pods created by controllers or in race conditions.
func getWorkloadResourcePodsLink(opts *Options, ownerRefs map[string][]OwnerRefInfo, deploymentStart string, deploymentEnd string) (string, error) {

	if len(ownerRefs) == 0 {
		return "", fmt.Errorf("no owner references found to build workload resource pods query")
	}

	if !isKustoConfigured(opts) {
		return "", nil
	}

	// Create "OR" string conditions for each owner reference to find all related pods
	var ownerConditions []string
	for namespace, owners := range ownerRefs {
		for _, ownerRef := range owners {
			ownerConditions = append(ownerConditions, fmt.Sprintf(`(pod_name startswith "%s" and namespace_name == "%s")`, ownerRef.Name, namespace))
		}
	}

	allOwnersQuery := fmt.Sprintf(`%s
| where ['time'] between (datetime("%s") .. datetime("%s"))
| where %s
| project ['time'], log, pod_name, namespace_name
| order by pod_name asc, ['time'] asc`,
		opts.KustoTable,
		deploymentStart,
		deploymentEnd,
		strings.Join(ownerConditions, " or "))

	return queryToDeepLink(allOwnersQuery, opts.KustoCluster, opts.KustoDatabase)
}

// Create a kusto query to retrieve all pods within the deployment namespace
func getNamespaceQuery(opts *Options, namespace string, deploymentStart string, deploymentEnd string) (string, error) {
	if !isKustoConfigured(opts) {
		return "", nil
	}
	// Create kusto deep link for entire namespace
	namespaceQuery := fmt.Sprintf(`%s
| where ['time'] between (datetime("%s") .. datetime("%s"))
| where namespace_name == "%s"
| project ['time'], log, pod_name
| order by pod_name asc, ['time'] asc`, opts.KustoTable, deploymentStart, deploymentEnd, namespace)

	return queryToDeepLink(namespaceQuery, opts.KustoCluster, opts.KustoDatabase)
}
