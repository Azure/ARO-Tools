package graph

import (
	"bytes"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/google/go-cmp/cmp"

	"k8s.io/apimachinery/pkg/util/sets"

	"github.com/Azure/ARO-Tools/pipelines/topology"
	"github.com/Azure/ARO-Tools/pipelines/types"
)

// Identifier records a dependency on a step in a particular service group and resource group.
// This is the minimum amount of precision required to identify a step in a multi-pipeline execution environment.
// Stamp is set by callers that expand stamped services into multiple graph copies — it distinguishes
// nodes that share the same ServiceGroup/ResourceGroup/Step but belong to different stamps.
type Identifier struct {
	Stamp        string
	ServiceGroup string
	types.StepDependency
}

func (i Identifier) ResourceGroupKey() ResourceGroupKey {
	return ResourceGroupKey{Stamp: i.Stamp, Name: i.ResourceGroup}
}

func (i Identifier) String() string {
	if i.Stamp != "" {
		return fmt.Sprintf("%s/%s/%s (stamp=%s)", i.ServiceGroup, i.ResourceGroup, i.Step, i.Stamp)
	}
	return fmt.Sprintf("%s/%s/%s", i.ServiceGroup, i.ResourceGroup, i.Step)
}

// Node records a step along with references to all parents and children. This structure is intentionally devoid of
// complex data, pointers to the underlying structures needed to execute the steps, etc. Such a structure helps to
// make operations that produce or operate over these nodes easy to test and verify.
type Node struct {
	// This embedded Identifier defines the identifier for this node.
	Identifier

	// Children contains the direct children (not further descendants) of this node.
	Children []Identifier
	// Parents contains the direct parents (not further ancestors) of this node.
	Parents []Identifier
}

// ResourceGroupKey identifies a resource group within a graph. Stamp is empty for
// unstamped resource groups. Callers that expand stamped services set Stamp to
// distinguish per-stamp resource group metadata that shares the same logical Name.
type ResourceGroupKey struct {
	Stamp string
	Name  string
}

func newGraph() *Graph {
	return &Graph{
		Services:               map[string]*topology.Service{},
		ResourceGroups:         map[ResourceGroupKey]*types.ResourceGroupMeta{},
		resourceGroupOwners:    map[ResourceGroupKey]sets.Set[string]{},
		Steps:                  map[Identifier]types.Step{},
		Nodes:                  []Node{},
		ServiceValidationSteps: map[Identifier]types.ValidationStep{},
	}
}

// Graph holds a set of nodes, recording parent/child relationships for each, along with a set of lookup tables for
// the services, resource groups, steps, etc. that the nodes represent.
type Graph struct {
	// Services is a lookup table of services by name (the service group).
	Services map[string]*topology.Service

	// ResourceGroups is a lookup table of resource groups keyed by stamp and logical name.
	// Unstamped resource groups use an empty Stamp value.
	ResourceGroups map[ResourceGroupKey]*types.ResourceGroupMeta

	// Subscription is an optional set of metadata required for subscription provisioning.
	Subscription *Subscription

	// Steps is a lookup table keyed by Identifier.
	Steps map[Identifier]types.Step

	// Nodes records every step, and the parent/child relationships between them.
	Nodes []Node

	// ServiceValidationSteps record the service validation steps
	ServiceValidationSteps map[Identifier]types.ValidationStep

	// resourceGroupOwners tracks which service groups have registered each resource group (internal book-keeping).
	resourceGroupOwners map[ResourceGroupKey]sets.Set[string]
}

// GetResourceGroup returns the resource group metadata for the given key.
func (c *Graph) GetResourceGroup(key ResourceGroupKey) (*types.ResourceGroupMeta, bool) {
	rg, exists := c.ResourceGroups[key]
	return rg, exists
}

// GetStep returns the step for a node, using the node's Stamp field to select per-stamp steps.
func (c *Graph) GetStep(node Identifier) (types.Step, bool) {
	s, exists := c.Steps[node]
	return s, exists
}

// Subscription holds the metadata required to handle subscription provisioning for an execution graph.
type Subscription struct {
	// ServiceGroup records the service which requested the subscription provisioning. In a multi-service execution graph, there may
	// be many services at play; subscription provisioning requires relative path resolution for role assignment ARM templates, so
	// the path to the service's directory must be known.
	ServiceGroup string

	// ResourceGroup records the semantic identifier for the resource group that requested subscription provisioning. The scope tags
	// used to parameterize the subscription provisioning must be tied to a specific resource group.
	ResourceGroup string

	Config types.SubscriptionProvisioning
}

// edge is a record of an inter-step dependency. This struct is unexported as we only use it during graph construction and do not
// expose edges directly as part of the graph.
type edge struct {
	from, to Identifier
}

// stampIteration pairs a stamp identifier with the pipeline to process for that stamp.
// Unstamped services use a single iteration with an empty stamp.
type stampIteration struct {
	stamp    string
	pipeline *types.Pipeline
}

// ForPipeline generates a graph for one pipeline, processing all steps therein to determine dependencies between them.
func ForPipeline(service *topology.Service, pipeline *types.Pipeline) (*Graph, error) {
	withoutChildren := &topology.Service{
		ServiceGroup: service.ServiceGroup,
		Purpose:      service.Purpose,
		PipelinePath: service.PipelinePath,
		Children:     nil, // explicitly omitted to generate graph for one pipeline only
		Metadata:     service.Metadata,
		Stamped:      nil, // one pipeline in, nothing to stamp-multiply
	}

	graph := newGraph()

	stampPipelines := map[string]map[string]*types.Pipeline{
		"": {pipeline.ServiceGroup: pipeline},
	}
	if err := graph.accumulate(withoutChildren, stampPipelines); err != nil {
		return nil, err
	}

	return graph, graph.detectCycles()
}

// ForEntrypoint generates a graph for all pipelines in the sub-tree of the topology identified by the entrypoint.
// Convenience wrapper around ForEntrypoints for a single entrypoint.
func ForEntrypoint(topo *topology.Topology, entrypoint *topology.Entrypoint, pipelines map[string]*types.Pipeline) (*Graph, error) {
	return ForEntrypoints(topo, []*topology.Entrypoint{entrypoint}, pipelines)
}

// ForEntrypoints generates a graph for all pipelines in the sub-trees of the topology identified by the entrypoints.
// Stamped services in the topology are not expanded — each appears exactly once with an empty Stamp field on its
// nodes. The resulting graph has one set of nodes per service, making it suitable for contexts where stamp expansion
// is handled by the runtime (e.g. EV2 rollout specs) rather than the graph itself.
func ForEntrypoints(topo *topology.Topology, entrypoints []*topology.Entrypoint, pipelines map[string]*types.Pipeline) (*Graph, error) {
	return forEntrypoints(topo, entrypoints, map[string]map[string]*types.Pipeline{"": pipelines})
}

// ForStampedEntrypoints generates a graph for all pipelines in the sub-trees of the topology identified by the
// entrypoints, expanding stamped services once per stamp. Each stamped service produces N copies of its nodes —
// one per non-empty key in stampPipelines — with each copy carrying a distinct Stamp field on its identifiers.
// Unstamped services appear once with an empty Stamp. The resulting graph is suitable for contexts where the graph
// itself drives per-stamp execution (e.g. templatize concurrent stamp rollouts).
func ForStampedEntrypoints(topo *topology.Topology, entrypoints []*topology.Entrypoint, stampPipelines map[string]map[string]*types.Pipeline) (*Graph, error) {
	return forEntrypoints(topo, entrypoints, stampPipelines)
}

func forEntrypoints(topo *topology.Topology, entrypoints []*topology.Entrypoint, stampPipelines map[string]map[string]*types.Pipeline) (*Graph, error) {
	var roots []*topology.Service
	for _, entrypoint := range entrypoints {
		root, err := topo.Lookup(entrypoint.Identifier)
		if err != nil {
			return nil, fmt.Errorf("failed to lookup entrypoint %s: %v", entrypoint.Identifier, err)
		}
		roots = append(roots, root)
	}

	graph := newGraph()

	for _, root := range roots {
		if err := graph.accumulate(root, stampPipelines); err != nil {
			return nil, err
		}
	}

	// External step dependencies break the nice separation between nodes of one pipeline and the rest of the graph,
	// so the nodesFor() method can no longer generate bi-directional edges as it does not see other nodes to add
	// child relations. Instead of trying to teach nodesFor() how to do half of these edges, we can do a pass
	// now with full context.
	if err := graph.addExternalDependencyEdges(); err != nil {
		return nil, err
	}

	return graph, graph.detectCycles()
}

// accumulate recursively traverses the service and all children, building a graph of how steps in each
// service depend on each other. Stamped services are expanded once per stamp; unstamped services once.
func (c *Graph) accumulate(service *topology.Service, stampPipelines map[string]map[string]*types.Pipeline) error {
	if _, alreadyRecorded := c.Services[service.ServiceGroup]; alreadyRecorded {
		return fmt.Errorf("service group %s already recorded", service.ServiceGroup)
	}
	c.Services[service.ServiceGroup] = service

	iterations, err := resolveIterations(service, stampPipelines)
	if err != nil {
		return err
	}

	// Leaf nodes are collected per stamp so that inter-service wiring connects only matching stamps.
	allLeaves := map[string][]Identifier{}
	for _, iter := range iterations {
		leaves, err := c.accumulateIteration(service.ServiceGroup, iter)
		if err != nil {
			return err
		}
		allLeaves[iter.stamp] = leaves
	}

	// Wire inter-service edges: connect this service's leaves to child service roots.
	for _, child := range service.Children {
		if err := c.accumulate(&child, stampPipelines); err != nil {
			return err
		}
		if err := c.wireInterServiceEdges(service, &child, allLeaves); err != nil {
			return err
		}
	}

	return nil
}

// resolveIterations determines which stamp/pipeline pairs to process for a service.
// Unstamped services run once with an empty stamp. Stamped services expand once per
// non-empty stamp key.
func resolveIterations(service *topology.Service, stampPipelines map[string]map[string]*types.Pipeline) ([]stampIteration, error) {
	// Unstamped services use the same pipeline regardless of stamp — grab from any entry.
	if !service.IsStamped() {
		for _, pipelines := range stampPipelines {
			if p, exists := pipelines[service.ServiceGroup]; exists {
				return []stampIteration{{pipeline: p}}, nil
			}
		}
		return nil, fmt.Errorf("pipeline for service %s not found", service.ServiceGroup)
	}

	var stamps []string
	for stamp := range stampPipelines {
		if stamp != "" {
			stamps = append(stamps, stamp)
		}
	}
	slices.Sort(stamps)

	// No non-empty stamps: fall back to single iteration without expansion.
	if len(stamps) == 0 {
		for _, pipelines := range stampPipelines {
			if p, exists := pipelines[service.ServiceGroup]; exists {
				return []stampIteration{{pipeline: p}}, nil
			}
		}
		return nil, fmt.Errorf("pipeline for service %s not found", service.ServiceGroup)
	}

	var iterations []stampIteration
	for _, stamp := range stamps {
		pipeline, exists := stampPipelines[stamp][service.ServiceGroup]
		if !exists {
			return nil, fmt.Errorf("pipeline for service %s not found in stamp %s", service.ServiceGroup, stamp)
		}
		iterations = append(iterations, stampIteration{stamp: stamp, pipeline: pipeline})
	}
	return iterations, nil
}

// accumulateIteration processes one stamp iteration: generates nodes, registers resource groups,
// steps, and metadata, and returns the leaf nodes for inter-service wiring.
func (c *Graph) accumulateIteration(serviceGroup string, iter stampIteration) ([]Identifier, error) {
	resourceGroups, subscription, steps, serviceValidationSteps, nodes, err := nodesFor(iter.pipeline, iter.stamp)
	if err != nil {
		return nil, fmt.Errorf("failed to generate graph for pipeline %s: %v", serviceGroup, err)
	}

	if err := c.registerResourceGroups(serviceGroup, iter.stamp, resourceGroups); err != nil {
		return nil, err
	}

	if subscription != nil && c.Subscription != nil {
		return nil, fmt.Errorf("subscription provisioning already recorded for %s/%s, cannot add another for %s/%s", c.Subscription.ServiceGroup, c.Subscription.ResourceGroup, subscription.ServiceGroup, subscription.ResourceGroup)
	}
	if subscription != nil {
		c.Subscription = subscription
	}

	// Register steps and validation steps with stamp-qualified identifiers.
	for rg, stepMap := range steps {
		for stepName, step := range stepMap {
			c.Steps[Identifier{Stamp: iter.stamp, ServiceGroup: serviceGroup, StepDependency: types.StepDependency{ResourceGroup: rg, Step: stepName}}] = step
		}
	}
	maps.Copy(c.ServiceValidationSteps, serviceValidationSteps)
	c.Nodes = append(c.Nodes, nodes...)

	return c.findLeaves(nodes)
}

// registerResourceGroups records resource groups, detecting conflicts across services and stamps.
func (c *Graph) registerResourceGroups(serviceGroup, stamp string, resourceGroups map[string]*types.ResourceGroupMeta) error {
	for name, group := range resourceGroups {
		key := ResourceGroupKey{Stamp: stamp, Name: name}
		other, alreadyRecorded := c.ResourceGroups[key]
		if alreadyRecorded && !resourceGroupMetaEqual(group, other) {
			existingOwners := sets.List(c.resourceGroupOwners[key])
			return fmt.Errorf("resource group %s already recorded with different step meta (existing services: %s, new service: %s), diff: %v", name, strings.Join(existingOwners, ", "), serviceGroup, cmp.Diff(group, other))
		}
		if !alreadyRecorded {
			c.ResourceGroups[key] = group
			c.resourceGroupOwners[key] = sets.New[string]()
		}
		c.resourceGroupOwners[key].Insert(serviceGroup)
	}
	return nil
}

// findLeaves identifies leaf nodes for a stamp iteration — these will become parents of child service roots.
// A node is a leaf when it is considered for service group completion and none of its children are.
func (c *Graph) findLeaves(nodes []Node) ([]Identifier, error) {
	var leaves []Identifier
	for _, node := range nodes {
		_, _, step, err := c.lookup(node.Identifier)
		if err != nil {
			return nil, fmt.Errorf("failed to lookup node: %v", err)
		}
		if !step.ConsideredForServiceGroupCompletion() {
			continue
		}
		has, err := c.hasCompletionChild(node)
		if err != nil {
			return nil, err
		}
		if has {
			continue
		}
		leaves = append(leaves, node.Identifier)
	}
	return leaves, nil
}

func (c *Graph) hasCompletionChild(node Node) (bool, error) {
	for _, child := range node.Children {
		_, _, childStep, err := c.lookup(child)
		if err != nil {
			return false, fmt.Errorf("failed to lookup child node %s of %s: %w", child, node.Identifier, err)
		}
		if childStep.ConsideredForServiceGroupCompletion() {
			return true, nil
		}
	}
	return false, nil
}

// wireInterServiceEdges connects parent service leaves to child service roots.
//
// The data we're using to build this graph come in two levels of granularity:
//   - specific, intra-service step relationships defined in a pipeline
//   - granular, inter-service relationships defined in the topology
//
// accumulate() will have built a sub-graph of step nodes for the specific child service,
// which we now need to decorate to record that all steps in that child depend on the parent service.
// There is no defined "end" to a pipeline, nor a "start", as each service may itself be a forest - having
// many roots and many leaves. Therefore, the simplest approach here is to record that every root node
// of the child depends on all the leaf nodes of the parent service, and vice versa.
//
// When stamps are involved, wiring is stamp-scoped: stamp-1 leaves connect to stamp-1 roots only.
// Unstamped leaves (stamp="") connect to all child roots that share the same stamp="" or fan out
// to all stamps when the child is stamped and the parent is not.
func (c *Graph) wireInterServiceEdges(parent *topology.Service, child *topology.Service, allLeaves map[string][]Identifier) error {
	for parentStamp, leaves := range allLeaves {
		roots := c.findChildRoots(parent, child, parentStamp)

		for i, node := range c.Nodes {
			if !slices.Contains(roots, node.Identifier) {
				continue
			}
			c.Nodes[i].Parents = append(c.Nodes[i].Parents, leaves...)
		}

		for _, leaf := range leaves {
			idx, err := c.node(leaf)
			if err != nil {
				return fmt.Errorf("failed to find leaf node: %v", err)
			}
			c.Nodes[idx].Children = append(c.Nodes[idx].Children, roots...)
		}
	}
	return nil
}

// findChildRoots returns identifiers of root nodes (no parents) for the child service.
// Our topology allows each service to depend on one and only one parent, so len(node.Parents) == 0
// safely identifies root nodes, and this is the only time any actor will add parents to these roots.
// When both parent and child are stamped, only matching-stamp roots are returned.
func (c *Graph) findChildRoots(parent, child *topology.Service, parentStamp string) []Identifier {
	var roots []Identifier
	for _, node := range c.Nodes {
		if node.ServiceGroup != child.ServiceGroup || len(node.Parents) != 0 {
			continue
		}
		// When both parent and child are stamped, only wire matching stamps.
		// When parent is unstamped, all stamped child roots get these leaves (fan-out).
		if parent.IsStamped() && child.IsStamped() && node.Stamp != parentStamp {
			continue
		}
		roots = append(roots, node.Identifier)
	}
	return roots
}

func (c *Graph) lookup(node Identifier) (*topology.Service, *types.ResourceGroupMeta, types.Step, error) {
	svc, exists := c.Services[node.ServiceGroup]
	if !exists {
		return nil, nil, nil, fmt.Errorf("service %s does not exist", node.ServiceGroup)
	}
	resourceGroup, exists := c.GetResourceGroup(node.ResourceGroupKey())
	if !exists {
		return nil, nil, nil, fmt.Errorf("resource group %s for node %s does not exist", node.ResourceGroup, node)
	}
	step, exists := c.GetStep(node)
	if !exists {
		return nil, nil, nil, fmt.Errorf("step %s does not exist", node)
	}
	return svc, resourceGroup, step, nil
}

func (c *Graph) node(id Identifier) (int, error) {
	for i, node := range c.Nodes {
		if node.Identifier == id {
			return i, nil
		}
	}
	return 0, fmt.Errorf("node %s not found", id)
}

func (c *Graph) addExternalDependencyEdges() error {
	for i, node := range c.Nodes {
		step, ok := c.GetStep(node.Identifier)
		if !ok {
			return fmt.Errorf("step %s not found", node.Identifier)
		}
		external := step.ExternalDependencies()
		if len(external) == 0 {
			continue
		}
		for _, dep := range external {
			parent := Identifier{
				ServiceGroup: dep.ServiceGroup,
				StepDependency: types.StepDependency{
					ResourceGroup: dep.ResourceGroup,
					Step:          dep.Step,
				},
			}
			// Unstamped services cannot declare external dependencies on stamped services —
			// there is no single stamp to resolve to. When both sides are stamped, resolve
			// to the same stamp: stamp-1 work depends on stamp-1 of the target.
			if targetService := c.Services[dep.ServiceGroup]; targetService != nil && targetService.IsStamped() {
				sourceService := c.Services[node.ServiceGroup]
				if sourceService == nil || !sourceService.IsStamped() {
					return fmt.Errorf("unstamped node %s has an external dependency on stamped service %s — unstamped services cannot depend on stamped services", node.Identifier, dep.ServiceGroup)
				}
				parent.Stamp = node.Stamp
			}
			parentNodeIdx, err := c.node(parent)
			if err != nil {
				return err
			}
			parentNode := c.Nodes[parentNodeIdx]
			parentNode.Children = append(parentNode.Children, node.Identifier)
			slices.SortFunc(parentNode.Children, CompareDependencies)
			parentNode.Children = slices.Compact(parentNode.Children)
			c.Nodes[parentNodeIdx] = parentNode

			node.Parents = append(node.Parents, parent)
		}
		slices.SortFunc(node.Parents, CompareDependencies)
		node.Parents = slices.Compact(node.Parents)
		c.Nodes[i] = node
	}
	return nil
}

// nodesFor transforms a pipeline to the list of nodes and lookup tables required in a graph
func nodesFor(pipeline *types.Pipeline, stamp string) (
	map[string]*types.ResourceGroupMeta,
	*Subscription,
	map[string]map[string]types.Step,
	map[Identifier]types.ValidationStep,
	[]Node,
	error,
) {
	stepsByResourceGroupAndName := map[string]map[string]types.Step{}
	serviceValidationSteps := map[Identifier]types.ValidationStep{}
	resourceGroupsByName := map[string]*types.ResourceGroupMeta{}
	var subscription *Subscription
	for _, rg := range pipeline.ResourceGroups {
		resourceGroupsByName[rg.Name] = rg.ResourceGroupMeta
		if rg.SubscriptionProvisioning != nil && subscription != nil {
			return nil, nil, nil, nil, nil, fmt.Errorf("multiple subscriptions found for pipeline %s", pipeline.ServiceGroup)
		}
		if rg.SubscriptionProvisioning != nil {
			subscription = &Subscription{
				ServiceGroup:  pipeline.ServiceGroup,
				ResourceGroup: rg.Name,
				Config:        *rg.SubscriptionProvisioning,
			}
		}
		stepsByResourceGroupAndName[rg.Name] = map[string]types.Step{}
		for _, step := range rg.Steps {
			stepsByResourceGroupAndName[rg.Name][step.StepName()] = step
		}
		for _, step := range rg.ValidationSteps {
			serviceValidationSteps[Identifier{
				Stamp:        stamp,
				ServiceGroup: pipeline.ServiceGroup,
				StepDependency: types.StepDependency{
					ResourceGroup: rg.Name,
					Step:          step.StepName(),
				},
			}] = step
		}
	}

	var stepDependencies []edge
	for _, rg := range pipeline.ResourceGroups {
		for _, step := range rg.Steps {
			dependsOn := append(step.Dependencies(), step.RequiredInputs()...)
			slices.SortFunc(dependsOn, CompareStepDependencies)
			dependsOn = slices.Compact(dependsOn)

			for _, dep := range dependsOn {
				stepDependencies = append(stepDependencies, edge{
					from: Identifier{
						Stamp:        stamp,
						ServiceGroup: pipeline.ServiceGroup,
						StepDependency: types.StepDependency{
							ResourceGroup: dep.ResourceGroup,
							Step:          dep.Step,
						},
					},
					to: Identifier{
						Stamp:        stamp,
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

	childrenOf := map[Identifier][]Identifier{}
	parentsOf := map[Identifier][]Identifier{}
	for _, e := range stepDependencies {
		childrenOf[e.from] = append(childrenOf[e.from], e.to)
		parentsOf[e.to] = append(parentsOf[e.to], e.from)
	}

	var nodes []Node
	for resourceGroup, steps := range stepsByResourceGroupAndName {
		for stepName := range steps {
			id := Identifier{
				Stamp:        stamp,
				ServiceGroup: pipeline.ServiceGroup,
				StepDependency: types.StepDependency{
					ResourceGroup: resourceGroup,
					Step:          stepName,
				},
			}
			nodes = append(nodes, Node{
				Identifier: id,
				Children:   childrenOf[id],
				Parents:    parentsOf[id],
			})
		}
	}

	slices.SortFunc(nodes, func(a, b Node) int {
		return CompareDependencies(a.Identifier, b.Identifier)
	})

	return resourceGroupsByName, subscription, stepsByResourceGroupAndName, serviceValidationSteps, nodes, nil
}

func CompareDependencies(a, b Identifier) int {
	if comparison := strings.Compare(a.Stamp, b.Stamp); comparison != 0 {
		return comparison
	}
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
	for i, ac := range a.ExecutionConstraints {
		bc := b.ExecutionConstraints[i]
		if ac.Singleton != bc.Singleton {
			return false
		}
		if !sets.New(ac.Clouds...).Equal(sets.New(bc.Clouds...)) {
			return false
		}
		if !sets.New(ac.Environments...).Equal(sets.New(bc.Environments...)) {
			return false
		}
		if !sets.New(ac.Regions...).Equal(sets.New(bc.Regions...)) {
			return false
		}
	}

	return true
}

// detectCycles runs a depth-first traversal of the tree, starting at every node, to detect cycles
func (c *Graph) detectCycles() error {
	nodesByID := make(map[Identifier]Node, len(c.Nodes))
	for _, node := range c.Nodes {
		nodesByID[node.Identifier] = node
	}
	for _, node := range c.Nodes {
		if err := traverse(node, nodesByID, []Identifier{node.Identifier}); err != nil {
			return err
		}
	}
	return nil
}

func traverse(node Node, nodesByID map[Identifier]Node, seen []Identifier) error {
	for _, child := range node.Children {
		if slices.Contains(seen, child) {
			var cycle []string
			for _, i := range seen {
				cycle = append(cycle, i.String())
			}
			return fmt.Errorf("cycle detected, reached %s via %s", child, strings.Join(cycle, " -> "))
		}
		childNode, found := nodesByID[child]
		if !found {
			return fmt.Errorf("could not find child node %s - programmer error", child)
		}
		if err := traverse(childNode, nodesByID, append(seen[:len(seen):len(seen)], child)); err != nil {
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

// MarshalDOT marshals the graph into the DOT notation used by the graphviz library.
// See documentation here: https://graphviz.gitlab.io/doc/info/lang.html
func MarshalDOT(g *Graph) ([]byte, error) {
	out := bytes.Buffer{}
	if n, err := out.WriteString(graphPrefix); err != nil || n != len(graphPrefix) {
		return nil, fmt.Errorf("failed to write graph prefix: wrote %d/%d bytes: %w", n, len(graphPrefix), err)
	}

	stampColors := buildStampColorMap(g.Nodes)

	for _, node := range g.Nodes {
		serviceGroup, err := shortenServiceGroup(node.ServiceGroup)
		if err != nil {
			return nil, err
		}

		nodeID := dotID(serviceGroup, node.Identifier)
		nodeLabel := dotLabel(serviceGroup, node.Identifier, g.ResourceGroups)
		attrs := fmt.Sprintf("label=\"%s\"", nodeLabel)
		if color, ok := stampColors[node.Stamp]; ok {
			attrs += fmt.Sprintf(" style=filled fillcolor=\"%s\"", color)
		}
		if _, err := fmt.Fprintf(&out, " \"%s\" [%s];\n", nodeID, attrs); err != nil {
			return nil, err
		}

		for _, child := range node.Children {
			childServiceGroup, err := shortenServiceGroup(child.ServiceGroup)
			if err != nil {
				return nil, err
			}

			childID := dotID(childServiceGroup, child)
			if _, err := fmt.Fprintf(&out, " \"%s\" -> \"%s\";\n", nodeID, childID); err != nil {
				return nil, err
			}
		}
	}

	for identifier := range g.ServiceValidationSteps {
		shortServiceGroup, err := shortenServiceGroup(identifier.ServiceGroup)
		if err != nil {
			return nil, err
		}
		if _, err := fmt.Fprintf(&out, " \"serviceValidation\" -> \"%s\";\n", dotID(shortServiceGroup, identifier)); err != nil {
			return nil, err
		}
	}

	if n, err := out.WriteString(graphSuffix); err != nil || n != len(graphSuffix) {
		return nil, fmt.Errorf("failed to write graph suffix: wrote %d/%d bytes: %w", n, len(graphSuffix), err)
	}
	return out.Bytes(), nil
}

var stampColorPalette = []string{
	"#B3D9FF", // light blue
	"#FFD9B3", // light orange
	"#B3FFB3", // light green
	"#FFB3D9", // light pink
	"#D9B3FF", // light purple
	"#FFFFB3", // light yellow
	"#B3FFFF", // light cyan
	"#FFB3B3", // light red
}

func buildStampColorMap(nodes []Node) map[string]string {
	stamps := sets.New[string]()
	for _, node := range nodes {
		if node.Stamp != "" {
			stamps.Insert(node.Stamp)
		}
	}
	sorted := sets.List(stamps)
	colors := make(map[string]string, len(sorted))
	for i, stamp := range sorted {
		colors[stamp] = stampColorPalette[i%len(stampColorPalette)]
	}
	return colors
}

func shortenServiceGroup(serviceGroup string) (string, error) {
	parts := strings.Split(serviceGroup, ".")
	if len(parts) < 5 {
		return "", fmt.Errorf("invalid service group: %q (expected at least 5 dot-separated parts, e.g. \"a.b.c.d.e\")", serviceGroup)
	}

	return strings.Join(parts[4:], "."), nil
}

func dotID(shortSG string, id Identifier) string {
	if id.Stamp != "" {
		return fmt.Sprintf("%s_%s_%s_%s", shortSG, id.ResourceGroup, id.Step, id.Stamp)
	}
	return fmt.Sprintf("%s_%s_%s", shortSG, id.ResourceGroup, id.Step)
}

func dotLabel(shortSG string, id Identifier, resourceGroups map[ResourceGroupKey]*types.ResourceGroupMeta) string {
	rgName := id.ResourceGroup
	if rg, ok := resourceGroups[id.ResourceGroupKey()]; ok {
		rgName = rg.ResourceGroup
	}
	if id.Stamp != "" {
		return fmt.Sprintf("%s/%s/%s (stamp=%s)", shortSG, rgName, id.Step, id.Stamp)
	}
	return fmt.Sprintf("%s/%s/%s", shortSG, rgName, id.Step)
}
