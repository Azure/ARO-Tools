package graph

import (
	"bytes"
	"embed"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/goccy/go-graphviz"

	"sigs.k8s.io/yaml"

	"github.com/Azure/ARO-Tools/pipelines/pkg/topology"
	"github.com/Azure/ARO-Tools/pipelines/pkg/types"
	"github.com/Azure/ARO-Tools/testutil"
)

//go:embed testdata/input
var testdata embed.FS

func TestForPipeline(t *testing.T) {
	_, _, service, pipelines := loadTestdata(t, "topology.yaml")

	pipeline, ok := pipelines[service.ServiceGroup]
	if !ok {
		t.Fatalf("pipeline %s not found", service.ServiceGroup)
	}

	ctx, err := ForPipeline(service, pipeline)
	if err != nil {
		t.Fatalf("Failed to create graph for pipeline: %v", err)
	}

	compareGraph(t, ctx.Nodes, ctx.ServiceValidationSteps)
}

func compareGraph(t *testing.T, nodes []Node, serviceValidationSteps map[Identifier]types.ValidationStep) {
	t.Helper()

	encoded, err := MarshalDOT(nodes, serviceValidationSteps)
	if err != nil {
		t.Fatalf("Failed to marshal graph: %v", err)
	}

	goldenFile := testutil.CompareWithFixture(t, encoded, testutil.WithExtension(".dot"))

	svgFile := strings.TrimSuffix(goldenFile, ".dot") + ".svg"

	cmd := exec.Command("dot", "-T", "svg", goldenFile, "-o", svgFile)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Failed to generate svg: %v; %s", err, string(out))
	}

	if false {
		// unfortunately, the Go bindings for the `graphviz` library fail on 1.24: https://github.com/goccy/go-graphviz/issues/112
		g, err := graphviz.New(t.Context())
		if err != nil {
			t.Fatalf("failed to create visualizer: %v", err)
		}

		graph, err := graphviz.ParseBytes(encoded)
		if err != nil {
			t.Fatalf("failed to decode graph: %v", err)
		}

		var image bytes.Buffer
		if err := g.Render(t.Context(), graph, graphviz.SVG, &image); err != nil {
			t.Fatalf("failed to: %v", err)
		}
		testutil.CompareWithFixture(t, image.Bytes(), testutil.WithExtension(".svg"))
	}

}

func TestForEntrypoint(t *testing.T) {
	topo, entrypoint, _, pipelines := loadTestdata(t, "topology.yaml")

	ctx, err := ForEntrypoint(topo, entrypoint, pipelines)
	if err != nil {
		t.Fatalf("Failed to create graph for entrypoint: %v", err)
	}

	compareGraph(t, ctx.Nodes, ctx.ServiceValidationSteps)
}

func TestForEntrypointDuplicateResourceGroups(t *testing.T) {
	topo, entrypoint, _, pipelines := loadTestdata(t, "duplicated-resourcegroup.topology.yaml")

	_, err := ForEntrypoint(topo, entrypoint, pipelines)
	if err == nil {
		t.Fatalf("expected duplicate resource group error, got nil")
	}
	if !strings.Contains(err.Error(), "already recorded with different step meta") {
		t.Fatalf("expected duplicate resource group error, got %v", err)
	}
	if !strings.Contains(err.Error(), "existing services:") {
		t.Fatalf("expected error to contain 'existing services:', got %v", err)
	}
	if !strings.Contains(err.Error(), "new service:") {
		t.Fatalf("expected error to contain 'new service:', got %v", err)
	}
}

func loadTestdata(t *testing.T, topologyPath string) (*topology.Topology, *topology.Entrypoint, *topology.Service, map[string]*types.Pipeline) {
	t.Helper()

	// for our sanity, we load and validate input test artifacts to make sure that changes to schema, etc.
	// are clearly caught and surfaced as test failures - this helps folks with future refactors
	rawTopology, err := testdata.ReadFile(filepath.Join("testdata", "input", topologyPath))
	if err != nil {
		t.Fatalf("Failed to read topology file: %v", err)
	}
	var topo topology.Topology
	if err := yaml.Unmarshal(rawTopology, &topo); err != nil {
		t.Fatalf("Failed to unmarshal topology: %v", err)
	}
	if err := topo.Validate(); err != nil {
		t.Fatalf("Failed to validate topology: %v", err)
	}

	if len(topo.Entrypoints) == 0 {
		t.Fatalf("Topology should have at least one entrypoint")
	}

	entrypoint := topo.Entrypoints[0]
	service, err := topo.Lookup(entrypoint.Identifier)
	if err != nil {
		t.Fatalf("Failed to lookup entrypoint: %v", err)
	}

	pipelines := map[string]*types.Pipeline{}
	if err := loadPipelines(service, pipelines); err != nil {
		t.Fatalf("Failed to load pipelines: %v", err)
	}

	return &topo, &entrypoint, service, pipelines
}

func loadPipelines(root *topology.Service, pipelines map[string]*types.Pipeline) error {
	file := filepath.Join("testdata", "input", root.PipelinePath)
	pipelineBytes, err := testdata.ReadFile(file)
	if err != nil {
		return fmt.Errorf("failed to read pipeline file %s: %v", file, err)
	}

	pipeline, err := types.NewPipelineFromBytes(pipelineBytes, map[string]any{})
	if err != nil {
		return fmt.Errorf("failed to parse pipeline file %s: %v", file, err)
	}

	pipelines[root.ServiceGroup] = pipeline

	for _, child := range root.Children {
		if err := loadPipelines(&child, pipelines); err != nil {
			return err
		}
	}
	return nil
}
