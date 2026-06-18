package graph

import (
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/Azure/ARO-Tools/pipelines/topology"
	"github.com/Azure/ARO-Tools/pipelines/types"
	"github.com/Azure/ARO-Tools/testutil"
)

func ptr[T any](v T) *T { return &v }

// nodesForSG filters graph nodes to those belonging to the given service group.
func nodesForSG(result *Graph, serviceGroup string) []Node {
	var nodes []Node
	for _, node := range result.Nodes {
		if node.ServiceGroup == serviceGroup {
			nodes = append(nodes, node)
		}
	}
	return nodes
}

// makeTopology builds a topology from a root service, registers it as the sole entrypoint, and propagates stamp flags.
func makeTopology(root topology.Service) *topology.Topology {
	topo := &topology.Topology{
		Services:    []topology.Service{root},
		Entrypoints: []topology.Entrypoint{{Identifier: root.ServiceGroup}},
	}
	topo.PropagateStamped()
	return topo
}

// makePipeline builds a single-resource-group pipeline with auto-derived RG metadata.
func makePipeline(serviceGroup, rgName string, steps ...types.Step) *types.Pipeline {
	return &types.Pipeline{
		ServiceGroup: serviceGroup,
		ResourceGroups: []*types.ResourceGroup{
			{
				ResourceGroupMeta: &types.ResourceGroupMeta{
					Name:          rgName,
					ResourceGroup: rgName,
					Subscription:  "sub-" + rgName,
				},
				Steps: steps,
			},
		},
	}
}

// makePipelineWithRGMeta builds a single-resource-group pipeline with explicit RG metadata (e.g. per-stamp subscriptions).
func makePipelineWithRGMeta(serviceGroup string, rgMeta *types.ResourceGroupMeta, steps ...types.Step) *types.Pipeline {
	return &types.Pipeline{
		ServiceGroup: serviceGroup,
		ResourceGroups: []*types.ResourceGroup{
			{
				ResourceGroupMeta: rgMeta,
				Steps:             steps,
			},
		},
	}
}

// makePipelineWithValidation builds a pipeline with both regular steps and validation steps.
func makePipelineWithValidation(serviceGroup string, rgMeta *types.ResourceGroupMeta, validationSteps []types.ValidationStep, steps ...types.Step) *types.Pipeline {
	return &types.Pipeline{
		ServiceGroup: serviceGroup,
		ResourceGroups: []*types.ResourceGroup{
			{
				ResourceGroupMeta: rgMeta,
				Steps:             steps,
				ValidationSteps:   validationSteps,
			},
		},
	}
}

// stampSets extracts and sorts the stamp values from a slice of nodes for comparison.
func stampSets(nodes []Node) []string {
	var stamps []string
	for _, node := range nodes {
		stamps = append(stamps, node.Stamp)
	}
	slices.Sort(stamps)
	return stamps
}

// buildAndValidate constructs a graph via ForEntrypoints and runs the validation function against it.
func buildAndValidate(t *testing.T, topo *topology.Topology, stampPipelines map[string]map[string]*types.Pipeline, validate func(t *testing.T, result *Graph)) {
	t.Helper()
	entrypoint := &topo.Entrypoints[0]
	result, err := ForEntrypoints(topo, []*topology.Entrypoint{entrypoint}, stampPipelines)
	assert.NoError(t, err)
	validate(t, result)
}

// TestStampedUnstampedParent tests stamped graph construction with:
//
//	SG.Infra (unstamped) → SG.Mgmt (stamped)
func TestStampedUnstampedParent(t *testing.T) {
	deploy := &types.ShellStep{StepMeta: types.StepMeta{Name: "deploy"}}

	topo := makeTopology(topology.Service{
		ServiceGroup: "SG.Infra", Purpose: "infra", PipelinePath: "infra.yaml",
		Children: []topology.Service{
			{ServiceGroup: "SG.Mgmt", Purpose: "mgmt", Stamped: ptr(true), PipelinePath: "mgmt.yaml"},
		},
	})

	infraPipeline := makePipeline("SG.Infra", "infra-rg", deploy)

	twoStampPipelines := map[string]map[string]*types.Pipeline{
		"1": {
			"SG.Infra": infraPipeline,
			"SG.Mgmt":  makePipelineWithRGMeta("SG.Mgmt", &types.ResourceGroupMeta{Name: "mgmt-rg", ResourceGroup: "mgmt-rg-1", Subscription: "sub-1"}, deploy),
		},
		"2": {
			"SG.Infra": infraPipeline,
			"SG.Mgmt":  makePipelineWithRGMeta("SG.Mgmt", &types.ResourceGroupMeta{Name: "mgmt-rg", ResourceGroup: "mgmt-rg-2", Subscription: "sub-2"}, deploy),
		},
	}

	testCases := []struct {
		name           string
		stampPipelines map[string]map[string]*types.Pipeline
		validate       func(t *testing.T, result *Graph)
	}{
		{
			name:           "unstamped nodes appear once across stamps",
			stampPipelines: twoStampPipelines,
			validate: func(t *testing.T, result *Graph) {
				infraNodes := nodesForSG(result, "SG.Infra")
				assert.Equal(t, 1, len(infraNodes))
				assert.Empty(t, infraNodes[0].Stamp)
			},
		},
		{
			name:           "stamped nodes appear once per stamp",
			stampPipelines: twoStampPipelines,
			validate: func(t *testing.T, result *Graph) {
				mgmtNodes := nodesForSG(result, "SG.Mgmt")
				assert.Equal(t, 2, len(mgmtNodes))
				assert.Equal(t, []string{"1", "2"}, stampSets(mgmtNodes))
			},
		},
		{
			name:           "stamped nodes reference unstamped parent",
			stampPipelines: twoStampPipelines,
			validate: func(t *testing.T, result *Graph) {
				infraID := Identifier{ServiceGroup: "SG.Infra", StepDependency: types.StepDependency{ResourceGroup: "infra-rg", Step: "deploy"}}
				for _, node := range nodesForSG(result, "SG.Mgmt") {
					assert.Equal(t, []Identifier{infraID}, node.Parents)
				}
			},
		},
		{
			name:           "unstamped parent has one child per stamp",
			stampPipelines: twoStampPipelines,
			validate: func(t *testing.T, result *Graph) {
				infraNode := nodesForSG(result, "SG.Infra")[0]
				var childStamps []string
				for _, child := range infraNode.Children {
					assert.Equal(t, "SG.Mgmt", child.ServiceGroup)
					childStamps = append(childStamps, child.Stamp)
				}
				slices.Sort(childStamps)
				assert.Equal(t, []string{"1", "2"}, childStamps)
			},
		},
		{
			name:           "no stale unstamped child references on parent",
			stampPipelines: twoStampPipelines,
			validate: func(t *testing.T, result *Graph) {
				for _, child := range nodesForSG(result, "SG.Infra")[0].Children {
					if child.ServiceGroup == "SG.Mgmt" {
						assert.NotEmpty(t, child.Stamp)
					}
				}
			},
		},
		{
			name:           "resource groups keyed by stamp for stamped services",
			stampPipelines: twoStampPipelines,
			validate: func(t *testing.T, result *Graph) {
				_, ok := result.ResourceGroups[ResourceGroupKey{Name: "infra-rg"}]
				assert.True(t, ok, "unstamped RG present")

				_, ok = result.ResourceGroups[ResourceGroupKey{Name: "mgmt-rg"}]
				assert.False(t, ok, "no unstamped mgmt-rg")

				rg1 := result.ResourceGroups[ResourceGroupKey{Stamp: "1", Name: "mgmt-rg"}]
				assert.Equal(t, "mgmt-rg-1", rg1.ResourceGroup)

				rg2 := result.ResourceGroups[ResourceGroupKey{Stamp: "2", Name: "mgmt-rg"}]
				assert.Equal(t, "mgmt-rg-2", rg2.ResourceGroup)
			},
		},
		{
			name:           "GetResourceGroup resolves stamped and unstamped keys",
			stampPipelines: twoStampPipelines,
			validate: func(t *testing.T, result *Graph) {
				rg, ok := result.GetResourceGroup(ResourceGroupKey{Name: "infra-rg"})
				assert.True(t, ok)
				assert.Equal(t, "infra-rg", rg.ResourceGroup)

				rg, ok = result.GetResourceGroup(ResourceGroupKey{Stamp: "1", Name: "mgmt-rg"})
				assert.True(t, ok)
				assert.Equal(t, "mgmt-rg-1", rg.ResourceGroup)

				_, ok = result.GetResourceGroup(ResourceGroupKey{Stamp: "99", Name: "mgmt-rg"})
				assert.False(t, ok)
			},
		},
		{
			name: "single stamp graph is valid",
			stampPipelines: map[string]map[string]*types.Pipeline{
				"1": {
					"SG.Infra": infraPipeline,
					"SG.Mgmt":  makePipelineWithRGMeta("SG.Mgmt", &types.ResourceGroupMeta{Name: "mgmt-rg", ResourceGroup: "mgmt-rg-1", Subscription: "sub-1"}, deploy),
				},
			},
			validate: func(t *testing.T, result *Graph) {
				assert.Equal(t, 2, len(result.Nodes))
				infraNode := nodesForSG(result, "SG.Infra")[0]
				assert.Empty(t, infraNode.Stamp)
				assert.Equal(t, 1, len(infraNode.Children))
				assert.Equal(t, "1", infraNode.Children[0].Stamp)

				mgmtNode := nodesForSG(result, "SG.Mgmt")[0]
				assert.Equal(t, "1", mgmtNode.Stamp)
			},
		},
		{
			name:           "step lookups work for stamped and unstamped",
			stampPipelines: twoStampPipelines,
			validate: func(t *testing.T, result *Graph) {
				_, ok := result.GetStep(Identifier{ServiceGroup: "SG.Infra", StepDependency: types.StepDependency{ResourceGroup: "infra-rg", Step: "deploy"}})
				assert.True(t, ok, "unstamped step")

				_, ok = result.GetStep(Identifier{Stamp: "1", ServiceGroup: "SG.Mgmt", StepDependency: types.StepDependency{ResourceGroup: "mgmt-rg", Step: "deploy"}})
				assert.True(t, ok, "stamped step")
			},
		},
		{
			name: "validation steps keyed per stamp",
			stampPipelines: func() map[string]map[string]*types.Pipeline {
				valStep := &types.GenericValidationStep{StepMeta: types.StepMeta{Name: "validate"}}
				return map[string]map[string]*types.Pipeline{
					"1": {
						"SG.Infra": infraPipeline,
						"SG.Mgmt": makePipelineWithValidation("SG.Mgmt",
							&types.ResourceGroupMeta{Name: "mgmt-rg", ResourceGroup: "mgmt-rg-1", Subscription: "sub-1"},
							[]types.ValidationStep{valStep}, deploy),
					},
					"2": {
						"SG.Infra": infraPipeline,
						"SG.Mgmt": makePipelineWithValidation("SG.Mgmt",
							&types.ResourceGroupMeta{Name: "mgmt-rg", ResourceGroup: "mgmt-rg-2", Subscription: "sub-2"},
							[]types.ValidationStep{valStep}, deploy),
					},
				}
			}(),
			validate: func(t *testing.T, result *Graph) {
				for _, stamp := range []string{"1", "2"} {
					key := Identifier{Stamp: stamp, ServiceGroup: "SG.Mgmt", StepDependency: types.StepDependency{ResourceGroup: "mgmt-rg", Step: "validate"}}
					assert.Contains(t, result.ServiceValidationSteps, key)
				}
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			buildAndValidate(t, topo, testCase.stampPipelines, testCase.validate)
		})
	}
}

// TestStampedMixedSiblings tests stamped graph construction with:
//
//	SG.Regional (unstamped) → SG.Svc (unstamped) + SG.Mgmt (stamped)
func TestStampedMixedSiblings(t *testing.T) {
	deploy := &types.ShellStep{StepMeta: types.StepMeta{Name: "deploy"}}

	topo := makeTopology(topology.Service{
		ServiceGroup: "SG.Regional", Purpose: "regional", PipelinePath: "regional.yaml",
		Children: []topology.Service{
			{ServiceGroup: "SG.Svc", Purpose: "svc", PipelinePath: "svc.yaml"},
			{ServiceGroup: "SG.Mgmt", Purpose: "mgmt", Stamped: ptr(true), PipelinePath: "mgmt.yaml"},
		},
	})

	regionalPipeline := makePipeline("SG.Regional", "regional-rg", deploy)
	svcPipeline := makePipeline("SG.Svc", "svc-rg", deploy)

	stampPipelines := map[string]map[string]*types.Pipeline{
		"1": {
			"SG.Regional": regionalPipeline,
			"SG.Svc":      svcPipeline,
			"SG.Mgmt":     makePipelineWithRGMeta("SG.Mgmt", &types.ResourceGroupMeta{Name: "mgmt-rg", ResourceGroup: "mgmt-rg-1", Subscription: "sub-1"}, deploy),
		},
		"2": {
			"SG.Regional": regionalPipeline,
			"SG.Svc":      svcPipeline,
			"SG.Mgmt":     makePipelineWithRGMeta("SG.Mgmt", &types.ResourceGroupMeta{Name: "mgmt-rg", ResourceGroup: "mgmt-rg-2", Subscription: "sub-2"}, deploy),
		},
	}

	testCases := []struct {
		name     string
		validate func(t *testing.T, result *Graph)
	}{
		{
			name: "unstamped services appear once, stamped per stamp",
			validate: func(t *testing.T, result *Graph) {
				assert.Equal(t, 1, len(nodesForSG(result, "SG.Regional")))
				assert.Equal(t, 1, len(nodesForSG(result, "SG.Svc")))
				assert.Equal(t, 2, len(nodesForSG(result, "SG.Mgmt")))
			},
		},
		{
			name: "regional parent fans out to stamped mgmt and links to unstamped svc",
			validate: func(t *testing.T, result *Graph) {
				regionalNode := nodesForSG(result, "SG.Regional")[0]
				assert.Empty(t, regionalNode.Stamp)

				var svcChildren, mgmtChildren []Identifier
				for _, child := range regionalNode.Children {
					switch child.ServiceGroup {
					case "SG.Svc":
						svcChildren = append(svcChildren, child)
					case "SG.Mgmt":
						mgmtChildren = append(mgmtChildren, child)
					}
				}

				assert.Equal(t, 1, len(svcChildren))
				assert.Empty(t, svcChildren[0].Stamp)

				assert.Equal(t, 2, len(mgmtChildren))
				var mgmtChildStamps []string
				for _, child := range mgmtChildren {
					mgmtChildStamps = append(mgmtChildStamps, child.Stamp)
				}
				slices.Sort(mgmtChildStamps)
				assert.Equal(t, []string{"1", "2"}, mgmtChildStamps)
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			buildAndValidate(t, topo, stampPipelines, testCase.validate)
		})
	}
}

// TestStampedNestedChildren tests stamped graph construction with:
//
//	SG.Regional (unstamped) → SG.Mgmt (stamped) → SG.MgmtDB (stamped) + SG.MgmtNet (stamped)
func TestStampedNestedChildren(t *testing.T) {
	deploy := &types.ShellStep{StepMeta: types.StepMeta{Name: "deploy"}}

	topo := makeTopology(topology.Service{
		ServiceGroup: "SG.Regional", Purpose: "regional", PipelinePath: "regional.yaml",
		Children: []topology.Service{
			{ServiceGroup: "SG.Mgmt", Purpose: "mgmt", Stamped: ptr(true), PipelinePath: "mgmt.yaml",
				Children: []topology.Service{
					{ServiceGroup: "SG.MgmtDB", Purpose: "db", PipelinePath: "db.yaml"},
					{ServiceGroup: "SG.MgmtNet", Purpose: "net", PipelinePath: "net.yaml"},
				},
			},
		},
	})

	regionalPipeline := makePipeline("SG.Regional", "regional-rg", deploy)

	stampPipelines := map[string]map[string]*types.Pipeline{
		"1": {
			"SG.Regional": regionalPipeline,
			"SG.Mgmt":     makePipelineWithRGMeta("SG.Mgmt", &types.ResourceGroupMeta{Name: "mgmt-rg", ResourceGroup: "mgmt-rg-1", Subscription: "sub-1"}, deploy),
			"SG.MgmtDB":   makePipeline("SG.MgmtDB", "db-rg", deploy),
			"SG.MgmtNet":  makePipeline("SG.MgmtNet", "net-rg", deploy),
		},
		"2": {
			"SG.Regional": regionalPipeline,
			"SG.Mgmt":     makePipelineWithRGMeta("SG.Mgmt", &types.ResourceGroupMeta{Name: "mgmt-rg", ResourceGroup: "mgmt-rg-2", Subscription: "sub-2"}, deploy),
			"SG.MgmtDB":   makePipeline("SG.MgmtDB", "db-rg", deploy),
			"SG.MgmtNet":  makePipeline("SG.MgmtNet", "net-rg", deploy),
		},
	}

	testCases := []struct {
		name     string
		validate func(t *testing.T, result *Graph)
	}{
		{
			name: "node counts match topology shape",
			validate: func(t *testing.T, result *Graph) {
				assert.Equal(t, 1, len(nodesForSG(result, "SG.Regional")))
				assert.Equal(t, 2, len(nodesForSG(result, "SG.Mgmt")))
				assert.Equal(t, 2, len(nodesForSG(result, "SG.MgmtDB")))
				assert.Equal(t, 2, len(nodesForSG(result, "SG.MgmtNet")))
			},
		},
		{
			name: "stamped children wired to same stamp as parent",
			validate: func(t *testing.T, result *Graph) {
				for _, mgmtNode := range nodesForSG(result, "SG.Mgmt") {
					for _, child := range mgmtNode.Children {
						assert.Equal(t, mgmtNode.Stamp, child.Stamp,
							"parent stamp=%s child %s stamp=%s", mgmtNode.Stamp, child.ServiceGroup, child.Stamp)
					}
				}
			},
		},
		{
			name: "stamped children reference same-stamp parent",
			validate: func(t *testing.T, result *Graph) {
				for _, dbNode := range nodesForSG(result, "SG.MgmtDB") {
					assert.NotEmpty(t, dbNode.Stamp)
					for _, parent := range dbNode.Parents {
						assert.Equal(t, dbNode.Stamp, parent.Stamp)
					}
				}
			},
		},
		{
			name: "no cross-stamp wiring anywhere in graph",
			validate: func(t *testing.T, result *Graph) {
				for _, node := range result.Nodes {
					if node.Stamp == "" {
						continue
					}
					for _, child := range node.Children {
						if child.Stamp != "" {
							assert.Equal(t, node.Stamp, child.Stamp,
								"cross-stamp child: %s(stamp=%s) → %s(stamp=%s)",
								node.ServiceGroup, node.Stamp, child.ServiceGroup, child.Stamp)
						}
					}
					for _, parent := range node.Parents {
						if parent.Stamp != "" {
							assert.Equal(t, node.Stamp, parent.Stamp,
								"cross-stamp parent: %s(stamp=%s) ← %s(stamp=%s)",
								node.ServiceGroup, node.Stamp, parent.ServiceGroup, parent.Stamp)
						}
					}
				}
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			buildAndValidate(t, topo, stampPipelines, testCase.validate)
		})
	}
}

// TestStampedExternalDeps tests external dependency resolution across stamp boundaries.
func TestStampedExternalDeps(t *testing.T) {
	deploy := &types.ShellStep{StepMeta: types.StepMeta{Name: "deploy"}}

	testCases := []struct {
		name           string
		topo           *topology.Topology
		stampPipelines map[string]map[string]*types.Pipeline
		validate       func(t *testing.T, result *Graph)
	}{
		{
			name: "stamped to stamped resolves same stamp",
			topo: makeTopology(topology.Service{
				ServiceGroup: "SG.Regional", Purpose: "regional", PipelinePath: "regional.yaml",
				Children: []topology.Service{
					{ServiceGroup: "SG.Mgmt", Purpose: "mgmt", Stamped: ptr(true), PipelinePath: "mgmt.yaml",
						Children: []topology.Service{
							{ServiceGroup: "SG.MgmtDB", Purpose: "db", PipelinePath: "db.yaml"},
							{ServiceGroup: "SG.MgmtNet", Purpose: "net", PipelinePath: "net.yaml"},
						},
					},
				},
			}),
			stampPipelines: func() map[string]map[string]*types.Pipeline {
				netDeploy := &types.ShellStep{StepMeta: types.StepMeta{
					Name: "deploy",
					ExternalDependsOn: []types.ExternalStepDependency{
						{ServiceGroup: "SG.MgmtDB", StepDependency: types.StepDependency{ResourceGroup: "db-rg", Step: "deploy"}},
					},
				}}
				regionalPipeline := makePipeline("SG.Regional", "regional-rg", deploy)
				return map[string]map[string]*types.Pipeline{
					"1": {
						"SG.Regional": regionalPipeline,
						"SG.Mgmt":     makePipelineWithRGMeta("SG.Mgmt", &types.ResourceGroupMeta{Name: "mgmt-rg", ResourceGroup: "mgmt-rg-1", Subscription: "sub-1"}, deploy),
						"SG.MgmtDB":   makePipeline("SG.MgmtDB", "db-rg", deploy),
						"SG.MgmtNet":  makePipeline("SG.MgmtNet", "net-rg", netDeploy),
					},
					"2": {
						"SG.Regional": regionalPipeline,
						"SG.Mgmt":     makePipelineWithRGMeta("SG.Mgmt", &types.ResourceGroupMeta{Name: "mgmt-rg", ResourceGroup: "mgmt-rg-2", Subscription: "sub-2"}, deploy),
						"SG.MgmtDB":   makePipeline("SG.MgmtDB", "db-rg", deploy),
						"SG.MgmtNet":  makePipeline("SG.MgmtNet", "net-rg", netDeploy),
					},
				}
			}(),
			validate: func(t *testing.T, result *Graph) {
				for _, netNode := range nodesForSG(result, "SG.MgmtNet") {
					var dbParents []Identifier
					for _, parent := range netNode.Parents {
						if parent.ServiceGroup == "SG.MgmtDB" {
							dbParents = append(dbParents, parent)
						}
					}
					assert.Equal(t, 1, len(dbParents), "stamp %s net should have one db parent", netNode.Stamp)
					assert.Equal(t, netNode.Stamp, dbParents[0].Stamp, "external dep resolves to same stamp")
				}
			},
		},
		{
			name: "stamped to unstamped resolves without stamp",
			topo: makeTopology(topology.Service{
				ServiceGroup: "SG.Regional", Purpose: "regional", PipelinePath: "regional.yaml",
				Children: []topology.Service{
					{ServiceGroup: "SG.Svc", Purpose: "svc", PipelinePath: "svc.yaml"},
					{ServiceGroup: "SG.Mgmt", Purpose: "mgmt", Stamped: ptr(true), PipelinePath: "mgmt.yaml"},
				},
			}),
			stampPipelines: func() map[string]map[string]*types.Pipeline {
				mgmtDeploy := func(rgName string) *types.Pipeline {
					return makePipelineWithRGMeta("SG.Mgmt",
						&types.ResourceGroupMeta{Name: "mgmt-rg", ResourceGroup: rgName, Subscription: "sub-" + rgName},
						&types.ShellStep{StepMeta: types.StepMeta{
							Name: "deploy",
							ExternalDependsOn: []types.ExternalStepDependency{
								{ServiceGroup: "SG.Svc", StepDependency: types.StepDependency{ResourceGroup: "svc-rg", Step: "deploy"}},
							},
						}})
				}
				regionalPipeline := makePipeline("SG.Regional", "regional-rg", deploy)
				svcPipeline := makePipeline("SG.Svc", "svc-rg", deploy)
				return map[string]map[string]*types.Pipeline{
					"1": {"SG.Regional": regionalPipeline, "SG.Svc": svcPipeline, "SG.Mgmt": mgmtDeploy("mgmt-rg-1")},
					"2": {"SG.Regional": regionalPipeline, "SG.Svc": svcPipeline, "SG.Mgmt": mgmtDeploy("mgmt-rg-2")},
				}
			}(),
			validate: func(t *testing.T, result *Graph) {
				for _, mgmtNode := range nodesForSG(result, "SG.Mgmt") {
					var svcParents []Identifier
					for _, parent := range mgmtNode.Parents {
						if parent.ServiceGroup == "SG.Svc" {
							svcParents = append(svcParents, parent)
						}
					}
					assert.Equal(t, 1, len(svcParents), "stamp %s mgmt has one svc parent", mgmtNode.Stamp)
					assert.Empty(t, svcParents[0].Stamp, "external dep to unstamped service has empty stamp")
				}

				svcNode := nodesForSG(result, "SG.Svc")[0]
				var mgmtChildren []Identifier
				for _, child := range svcNode.Children {
					if child.ServiceGroup == "SG.Mgmt" {
						mgmtChildren = append(mgmtChildren, child)
					}
				}
				assert.Equal(t, 2, len(mgmtChildren), "unstamped svc has both stamped mgmt as children via external dep")
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			buildAndValidate(t, testCase.topo, testCase.stampPipelines, testCase.validate)
		})
	}
}

// TestDetectCyclesStamped verifies that cycle detection resolves child nodes by full
// Identifier (including Stamp) rather than only ServiceGroup/ResourceGroup/Step.
//
// Both stamps have the same cycle (A→B→A). If traverse matched nodes by only
// SG/RG/Step it would follow cross-stamp edges (A1→B2→A2→…) instead of the
// correct same-stamp path (A1→B1→A1), producing a misleading error.
func TestDetectCyclesStamped(t *testing.T) {
	idA1 := Identifier{Stamp: "1", ServiceGroup: "SG.A", StepDependency: types.StepDependency{ResourceGroup: "rg", Step: "deploy"}}
	idB1 := Identifier{Stamp: "1", ServiceGroup: "SG.B", StepDependency: types.StepDependency{ResourceGroup: "rg", Step: "deploy"}}
	idA2 := Identifier{Stamp: "2", ServiceGroup: "SG.A", StepDependency: types.StepDependency{ResourceGroup: "rg", Step: "deploy"}}
	idB2 := Identifier{Stamp: "2", ServiceGroup: "SG.B", StepDependency: types.StepDependency{ResourceGroup: "rg", Step: "deploy"}}

	graph := &Graph{
		Nodes: []Node{
			{Identifier: idA1, Children: []Identifier{idB1}},
			{Identifier: idB1, Children: []Identifier{idA1}},
			{Identifier: idA2, Children: []Identifier{idB2}},
			{Identifier: idB2, Children: []Identifier{idA2}},
		},
	}

	err := graph.detectCycles()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "cycle detected")

	msg := err.Error()
	hasStamp1 := strings.Contains(msg, "stamp=1")
	hasStamp2 := strings.Contains(msg, "stamp=2")
	assert.True(t, hasStamp1 || hasStamp2, "cycle path should include stamp info")
	assert.False(t, hasStamp1 && hasStamp2, "cycle path should stay within one stamp, got: %s", msg)
}

// TestStampedServiceRequiresNonEmptyStampKey verifies that stamped services fail loudly
// when no non-empty stamp key is provided.
func TestStampedServiceRequiresNonEmptyStampKey(t *testing.T) {
	deploy := &types.ShellStep{StepMeta: types.StepMeta{Name: "deploy"}}

	topo := makeTopology(topology.Service{
		ServiceGroup: "SG.Infra", Purpose: "infra", PipelinePath: "infra.yaml",
		Children: []topology.Service{
			{ServiceGroup: "SG.Mgmt", Purpose: "mgmt", Stamped: ptr(true), PipelinePath: "mgmt.yaml"},
		},
	})

	stampPipelines := map[string]map[string]*types.Pipeline{
		"": {
			"SG.Infra": makePipeline("SG.Infra", "infra-rg", deploy),
			"SG.Mgmt":  makePipeline("SG.Mgmt", "mgmt-rg", deploy),
		},
	}

	entrypoint := &topo.Entrypoints[0]
	_, err := ForEntrypoints(topo, []*topology.Entrypoint{entrypoint}, stampPipelines)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no non-empty stamp keys")
}

// TestForPipelineStampedService verifies that ForPipeline works with a stamped service,
// using a synthetic stamp key internally.
func TestForPipelineStampedService(t *testing.T) {
	deploy := &types.ShellStep{StepMeta: types.StepMeta{Name: "deploy"}}

	service := &topology.Service{
		ServiceGroup: "SG.Mgmt", Purpose: "mgmt", Stamped: ptr(true), PipelinePath: "mgmt.yaml",
	}
	pipeline := makePipeline("SG.Mgmt", "mgmt-rg", deploy)

	result, err := ForPipeline(service, pipeline)
	assert.NoError(t, err)
	assert.Equal(t, 1, len(result.Nodes))
	assert.Empty(t, result.Nodes[0].Stamp)
}

// TestStampedEntrypointDOT verifies MarshalDOT produces distinct node IDs and labels for
// stamped nodes across unstamped→stamped (parent-child) and stamped→stamped (external dep) edges.
func TestStampedEntrypointDOT(t *testing.T) {
	deploy := &types.ShellStep{StepMeta: types.StepMeta{Name: "deploy"}}

	netDeploy := &types.ShellStep{StepMeta: types.StepMeta{
		Name: "deploy",
		ExternalDependsOn: []types.ExternalStepDependency{
			{ServiceGroup: "Microsoft.Azure.ARO.HCP.MgmtDB", StepDependency: types.StepDependency{ResourceGroup: "db-rg", Step: "deploy"}},
		},
	}}

	topo := makeTopology(topology.Service{
		ServiceGroup: "Microsoft.Azure.ARO.HCP.Infra", Purpose: "infra", PipelinePath: "infra.yaml",
		Children: []topology.Service{
			{ServiceGroup: "Microsoft.Azure.ARO.HCP.Mgmt", Purpose: "mgmt", Stamped: ptr(true), PipelinePath: "mgmt.yaml",
				Children: []topology.Service{
					{ServiceGroup: "Microsoft.Azure.ARO.HCP.MgmtDB", Purpose: "db", PipelinePath: "db.yaml"},
					{ServiceGroup: "Microsoft.Azure.ARO.HCP.MgmtNet", Purpose: "net", PipelinePath: "net.yaml"},
				},
			},
		},
	})

	infraPipeline := makePipeline("Microsoft.Azure.ARO.HCP.Infra", "infra-rg", deploy)

	stampPipelines := map[string]map[string]*types.Pipeline{
		"1": {
			"Microsoft.Azure.ARO.HCP.Infra":   infraPipeline,
			"Microsoft.Azure.ARO.HCP.Mgmt":    makePipelineWithRGMeta("Microsoft.Azure.ARO.HCP.Mgmt", &types.ResourceGroupMeta{Name: "mgmt-rg", ResourceGroup: "mgmt-rg-1", Subscription: "sub-1"}, deploy),
			"Microsoft.Azure.ARO.HCP.MgmtDB":  makePipeline("Microsoft.Azure.ARO.HCP.MgmtDB", "db-rg", deploy),
			"Microsoft.Azure.ARO.HCP.MgmtNet": makePipeline("Microsoft.Azure.ARO.HCP.MgmtNet", "net-rg", netDeploy),
		},
		"2": {
			"Microsoft.Azure.ARO.HCP.Infra":   infraPipeline,
			"Microsoft.Azure.ARO.HCP.Mgmt":    makePipelineWithRGMeta("Microsoft.Azure.ARO.HCP.Mgmt", &types.ResourceGroupMeta{Name: "mgmt-rg", ResourceGroup: "mgmt-rg-2", Subscription: "sub-2"}, deploy),
			"Microsoft.Azure.ARO.HCP.MgmtDB":  makePipeline("Microsoft.Azure.ARO.HCP.MgmtDB", "db-rg", deploy),
			"Microsoft.Azure.ARO.HCP.MgmtNet": makePipeline("Microsoft.Azure.ARO.HCP.MgmtNet", "net-rg", netDeploy),
		},
	}

	entrypoint := &topo.Entrypoints[0]
	result, err := ForEntrypoints(topo, []*topology.Entrypoint{entrypoint}, stampPipelines)
	assert.NoError(t, err)

	encoded, err := MarshalDOT(result)
	assert.NoError(t, err)

	testutil.CompareWithFixture(t, encoded, testutil.WithExtension(".dot"))
}
