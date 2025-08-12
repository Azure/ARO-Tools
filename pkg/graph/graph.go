package graph

import (
	"bytes"
	"fmt"
	"slices"
	"strings"

	"github.com/google/go-cmp/cmp"

	"k8s.io/apimachinery/pkg/util/sets"

	"github.com/Azure/ARO-Tools/pkg/topology"
	"github.com/Azure/ARO-Tools/pkg/types"
)

// Dependency records a dependency on a step in a particular service group and resource group.
// This is the minimum amount of precision required to identify a step in a multi-pipeline execution environment.
type Dependency struct {
	ServiceGroup string
	types.StepDependency
}

// Node records a step along with references to all parents and children. This structure is intentionally devoid of
// complex data, pointers to the underlying structures needed to execute the steps, etc. Such a structure helps to
// make operations that produce or operate over these nodes easy to test and verify.
type Node struct {
	// This embedded Dependency defines the identifier for this node.
	Dependency

	// Children contains the direct children (not further descendants) of this node.
	Children []Dependency
	// Parents contains the direct parents (not further ancestors) of this node.
	Parents []Dependency
}

// Graph holds a set of nodes, recording parent/child relationships for each, along with a set of lookup tables for
// the services, resource groups, steps, etc. that the nodes represent.
type Graph struct {
	// Services is a lookup table of services by name (the service group).
	Services map[string]*topology.Service

	// ResourceGroups is a lookup table of resource group by name. We flatten the hierarchy of resource groups to
	// not require re-writing the dependency references in every step. Topologies that define more than one unique resource
	// group with the same identifier are disallowed.
	ResourceGroups map[string]*types.ResourceGroupMeta

	// Subscriptions is a lookup table of subscription provisioning metadata by resource group name.
	Subscriptions map[string]*types.SubscriptionProvisioning

	// Steps is a lookup table of service group -> resource group -> step name. Steps are *not* flattened, keeping record
	// of provenance and allowing step names to be kept short, unique only within their resource group.
	Steps map[string]map[string]map[string]types.Step

	// Nodes records every step, and the parent/child relationships between them.
	Nodes []Node
}

// edge is a record of an inter-step dependency. This struct is unexported as we only use it during graph construction and do not
// expose edges directly as part of the graph.
type edge struct {
	from, to Dependency
}

// ForPipeline generates a graph for one pipeline, processing all steps therein to determine dependencies between them.
func ForPipeline(service *topology.Service, pipeline *types.Pipeline) (*Graph, error) {
	withoutChildren := &topology.Service{
		ServiceGroup: service.ServiceGroup,
		Purpose:      service.Purpose,
		PipelinePath: service.PipelinePath,
		Children:     nil, // explicitly omitted to generate graph for one pipeline only
		Metadata:     service.Metadata,
	}

	graph := &Graph{
		Services:       map[string]*topology.Service{},
		ResourceGroups: map[string]*types.ResourceGroupMeta{},
		Steps:          map[string]map[string]map[string]types.Step{},
		Nodes:          []Node{},
	}

	if err := graph.accumulate(withoutChildren, map[string]*types.Pipeline{pipeline.ServiceGroup: pipeline}); err != nil {
		return nil, err
	}

	return graph, graph.detectCycles()
}

// ForEntrypoint generates a graph for all pipelines in the sub-tree of the topology identified by the entrypoint.
func ForEntrypoint(topo *topology.Topology, entrypoint *topology.Entrypoint, pipelines map[string]*types.Pipeline) (*Graph, error) {
	root, err := topo.Lookup(entrypoint.Identifier)
	if err != nil {
		return nil, fmt.Errorf("failed to lookup entrypoint %s: %v", entrypoint.Identifier, err)
	}

	graph := &Graph{
		Services:       map[string]*topology.Service{},
		ResourceGroups: map[string]*types.ResourceGroupMeta{},
		Steps:          map[string]map[string]map[string]types.Step{},
		Nodes:          []Node{},
	}

	if err := graph.accumulate(root, pipelines); err != nil {
		return nil, err
	}

	return graph, graph.detectCycles()
}

// accumulate recursively traverses the service and all children, building a graph of how steps in each service depend on each other.
func (c *Graph) accumulate(service *topology.Service, pipelines map[string]*types.Pipeline) error {
	if _, alreadyRecorded := c.Services[service.ServiceGroup]; alreadyRecorded {
		return fmt.Errorf("service group %s already recorded", service.ServiceGroup)
	}
	if _, alreadyRecorded := c.Steps[service.ServiceGroup]; alreadyRecorded {
		return fmt.Errorf("steps already recorded for service %s", service.ServiceGroup)
	}

	c.Services[service.ServiceGroup] = service
	c.Steps[service.ServiceGroup] = map[string]map[string]types.Step{}

	pipeline, exists := pipelines[service.ServiceGroup]
	if !exists {
		return fmt.Errorf("pipeline for service %s not found", service.ServiceGroup)
	}
	resourceGroups, subscriptions, steps, nodes := nodesFor(pipeline)
	for name, group := range resourceGroups {
		if other, alreadyRecorded := c.ResourceGroups[name]; alreadyRecorded && !resourceGroupMetaEqual(group, other) {
			return fmt.Errorf("resource group %s already recorded with different step meta, diff: %v", name, cmp.Diff(group, other))
		}
		c.ResourceGroups[name] = group
	}
	for name, sub := range subscriptions {
		if _, alreadyRecorded := c.Subscriptions[name]; alreadyRecorded {
			return fmt.Errorf("subscription %s already recorded", name)
		}
		c.Subscriptions[name] = sub
	}
	c.Steps[service.ServiceGroup] = steps
	c.Nodes = append(c.Nodes, nodes...)

	var leaves []Dependency
	for _, node := range c.Nodes {
		if len(node.Children) == 0 {
			leaves = append(leaves, node.Dependency)
		}
	}

	for _, child := range service.Children {
		if err := c.accumulate(&child, pipelines); err != nil {
			return err
		}

		// The data we're using to build this graph come in two levels of granularity:
		// - specific, intra-service step relationships defined in a pipeline
		// - granular, inter-service relationships defined in the topology
		// The above call to accumulate() will have build a sub-graph of step nodes for the specific child service,
		// which we now need to decorate to record that all steps in that child depend on the parent service.
		// There is no defined "end" to a pipeline, nor a "start", as each service may itself be a forest - having
		// many roots and many leaves. Therefore, the simplest approach here is to record that every root node
		// of the child depends on all the leaf nodes of the parent service, and vice versa.

		// First, find the root nodes for the child service, which accumulate() will have placed in the graph.
		// Record those roots for later use and mark them as being children of the leaves of the parent.
		var roots []Dependency
		for i, node := range c.Nodes {
			if node.ServiceGroup == child.ServiceGroup && len(node.Parents) == 0 {
				// accumulate() did not divulge the list of root nodes for that specific child, but we can find them
				roots = append(roots, node.Dependency)

				// our topology allows each service to depend on one and only one parent, so we know that
				// a) it's safe to determine that `len(node.Parents) == 0` identifies a root node for that service
				// b) this is the only time that any actor will add parents to this root node
				c.Nodes[i].Parents = append(c.Nodes[i].Parents, leaves...)
			}
		}

		// Second, using the identifiers in `leaves`, find the leaf nodes in the graph and mark them as having
		// the roots of the child service as children.
		for i, node := range c.Nodes {
			for _, leaf := range leaves {
				if node.ServiceGroup == leaf.ServiceGroup && node.ResourceGroup == leaf.ResourceGroup && node.Step == leaf.Step {
					c.Nodes[i].Children = append(c.Nodes[i].Children, roots...)
				}
			}
		}
	}

	return nil
}

// nodesFor transforms a pipeline to the list of nodes and lookup tables required in a graph
func nodesFor(pipeline *types.Pipeline) (map[string]*types.ResourceGroupMeta, map[string]*types.SubscriptionProvisioning, map[string]map[string]types.Step, []Node) {
	// first, create a registry of steps by their identifier (resource group name, step name)
	// and resource groups by name
	stepsByResourceGroupAndName := map[string]map[string]types.Step{}
	resourceGroupsByName := map[string]*types.ResourceGroupMeta{}
	subscriptionsByName := map[string]*types.SubscriptionProvisioning{}
	for _, rg := range pipeline.ResourceGroups {
		resourceGroupsByName[rg.Name] = rg.ResourceGroupMeta
		if rg.SubscriptionProvisioning != nil {
			subscriptionsByName[rg.Name] = rg.SubscriptionProvisioning
		}
		stepsByResourceGroupAndName[rg.Name] = map[string]types.Step{}
		for _, step := range rg.Steps {
			stepsByResourceGroupAndName[rg.Name][step.StepName()] = step
		}
	}

	// next, create an adjacency list of edges between these nodes
	var stepDependencies []edge
	for _, rg := range pipeline.ResourceGroups {
		for _, step := range rg.Steps {
			dependsOn := append(step.Dependencies(), step.RequiredInputs()...)
			slices.SortFunc(dependsOn, CompareStepDependencies)
			dependsOn = slices.Compact(dependsOn)

			for _, dep := range dependsOn {
				stepDependencies = append(stepDependencies, edge{
					from: Dependency{
						ServiceGroup: pipeline.ServiceGroup,
						StepDependency: types.StepDependency{
							ResourceGroup: dep.ResourceGroup,
							Step:          dep.Step,
						},
					},
					to: Dependency{
						ServiceGroup: pipeline.ServiceGroup,
						StepDependency: types.StepDependency{
							ResourceGroup: rg.Name,
							Step:          step.StepName(),
						},
					},
				})
			}
		}
	}

	slices.SortFunc(stepDependencies, func(a, b edge) int {
		if comparison := CompareDependencies(a.from, b.from); comparison != 0 {
			return comparison
		}
		return CompareDependencies(a.to, b.to)
	})

	// record edges as references in nodes for ease of traversal
	var nodes []Node
	for resourceGroup, steps := range stepsByResourceGroupAndName {
		for stepName := range steps {
			node := Node{
				Dependency: Dependency{
					ServiceGroup: pipeline.ServiceGroup,
					StepDependency: types.StepDependency{
						ResourceGroup: resourceGroup,
						Step:          stepName,
					},
				},
				Children: []Dependency{},
				Parents:  []Dependency{},
			}
			for _, edge := range stepDependencies {
				if edge.to.ServiceGroup == pipeline.ServiceGroup && edge.to.ResourceGroup == resourceGroup && edge.to.Step == stepName {
					node.Parents = append(node.Parents, edge.from)
				}
				if edge.from.ServiceGroup == pipeline.ServiceGroup && edge.from.ResourceGroup == resourceGroup && edge.from.Step == stepName {
					node.Children = append(node.Children, edge.to)
				}
			}
			nodes = append(nodes, node)
		}
	}

	slices.SortFunc(nodes, func(a, b Node) int {
		return CompareDependencies(a.Dependency, b.Dependency)
	})

	return resourceGroupsByName, subscriptionsByName, stepsByResourceGroupAndName, nodes
}

func CompareDependencies(a, b Dependency) int {
	if comparison := strings.Compare(a.ServiceGroup, b.ServiceGroup); comparison != 0 {
		return comparison
	}
	return CompareStepDependencies(a.StepDependency, b.StepDependency)
}

func CompareStepDependencies(a, b types.StepDependency) int {
	if comparison := strings.Compare(a.ResourceGroup, b.ResourceGroup); comparison != 0 {
		return comparison
	}
	return strings.Compare(a.Step, b.Step)
}

func resourceGroupMetaEqual(a, b *types.ResourceGroupMeta) bool {
	if a.Name != b.Name {
		return false
	}
	if a.ResourceGroup != b.ResourceGroup {
		return false
	}
	if a.Subscription != b.Subscription {
		return false
	}
	if len(a.ExecutionConstraints) != len(b.ExecutionConstraints) {
		return false
	}
	for i := 0; i < len(a.ExecutionConstraints); i++ {
		if a.ExecutionConstraints[i].Singleton != b.ExecutionConstraints[i].Singleton {
			return false
		}
		if !sets.New(a.ExecutionConstraints[i].Clouds...).Equal(sets.New(b.ExecutionConstraints[i].Clouds...)) {
			return false
		}
		if !sets.New(a.ExecutionConstraints[i].Environments...).Equal(sets.New(b.ExecutionConstraints[i].Environments...)) {
			return false
		}
		if !sets.New(a.ExecutionConstraints[i].Regions...).Equal(sets.New(b.ExecutionConstraints[i].Regions...)) {
			return false
		}
	}

	return true
}

// detectCycles runs a depth-first traversal of the tree, starting at every node, to detect cycles
func (c *Graph) detectCycles() error {
	for _, node := range c.Nodes {
		seen := []Dependency{
			node.Dependency,
		}
		if err := traverse(node, c.Nodes, seen); err != nil {
			return err
		}
	}
	return nil
}

func traverse(node Node, all []Node, seen []Dependency) error {
	for _, child := range node.Children {
		for _, previous := range seen {
			if previous == child {
				var cycle []string
				for _, i := range seen {
					cycle = append(cycle, fmt.Sprintf("%s/%s", i.ResourceGroup, i.Step))
				}
				return fmt.Errorf("cycle detected, reached %s/%s via %s", child.ResourceGroup, child.Step, strings.Join(cycle, " -> "))
			}
		}
		chain := seen[:]
		chain = append(chain, child)
		var childNode Node
		for _, candidate := range all {
			if candidate.ServiceGroup == child.ServiceGroup && candidate.ResourceGroup == child.ResourceGroup && candidate.Step == child.Step {
				childNode = candidate
			}
		}
		if childNode.ServiceGroup == "" {
			return fmt.Errorf("could not find child node %s/%s - programmer error", child.ResourceGroup, child.Step)
		}
		if err := traverse(childNode, all, chain); err != nil {
			return err
		}
	}
	return nil
}

const graphPrefix = `digraph regexp { 
 fontname="Helvetica,Arial,sans-serif"
 node [fontname="Helvetica,Arial,sans-serif"]
 edge [fontname="Helvetica,Arial,sans-serif"]
`

const graphSuffix = `}`

// MarshalDOT marshals the graph described by the list of nodes into the DOT notation used by the graphviz library.
// See documentation here: https://graphviz.gitlab.io/doc/info/lang.html
func MarshalDOT(nodes []Node) ([]byte, error) {
	out := bytes.Buffer{}
	if n, err := out.WriteString(graphPrefix); err != nil || n != len(graphPrefix) {
		return nil, fmt.Errorf("failed to write graph prefix: wrote %d/%d bytes: %w", n, len(graphPrefix), err)
	}

	for _, node := range nodes {
		serviceGroup, err := shortenServiceGroup(node.ServiceGroup)
		if err != nil {
			return nil, err
		}

		if _, err := out.WriteString(fmt.Sprintf(" \"%s_%s_%s\" [label=\"%s/%s/%s\"];\n", serviceGroup, node.ResourceGroup, node.Step, serviceGroup, node.ResourceGroup, node.Step)); err != nil {
			return nil, err
		}

		// n.b. we don't handle parent links, as they will be written by traversing children on the parent node
		for _, child := range node.Children {
			childServiceGroup, err := shortenServiceGroup(child.ServiceGroup)
			if err != nil {
				return nil, err
			}

			if _, err := out.WriteString(fmt.Sprintf(" \"%s_%s_%s\" -> \"%s_%s_%s\";\n", serviceGroup, node.ResourceGroup, node.Step, childServiceGroup, child.ResourceGroup, child.Step)); err != nil {
				return nil, err
			}
		}
	}

	if n, err := out.WriteString(graphSuffix); err != nil || n != len(graphSuffix) {
		return nil, fmt.Errorf("failed to write graph suffix: wrote %d/%d bytes: %w", n, len(graphSuffix), err)
	}
	return out.Bytes(), nil
}

func shortenServiceGroup(serviceGroup string) (string, error) {
	parts := strings.Split(serviceGroup, ".")
	if len(parts) < 5 {
		return "", fmt.Errorf("invalid service group: %q (expected at least 5 dot-separated parts, e.g. \"a.b.c.d.e\")", serviceGroup)
	}

	return strings.Join(parts[4:], "."), nil
}
