package topology

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/google/go-cmp/cmp"

	"sigs.k8s.io/yaml"
)

func TestValidate(t *testing.T) {
	for _, testCase := range []struct {
		name  string
		input string
		err   bool
	}{
		{
			name: "happy path",
			input: `entrypoints:
- identifier: Microsoft.Azure.ARO.HCP.Whatever.Child
services:
- serviceGroup: Microsoft.Azure.ARO.HCP
  pipelinePath: foo
  purpose: stuff
  children:
  - serviceGroup: Microsoft.Azure.ARO.HCP.Whatever
    pipelinePath: foo
    purpose: stuff
    children:
    - serviceGroup: Microsoft.Azure.ARO.HCP.Whatever.Child
      pipelinePath: foo
      purpose: stuff
  - serviceGroup: Microsoft.Azure.ARO.HCP.Other
    pipelinePath: foo
    purpose: stuff
- serviceGroup: Microsoft.Azure.ARO.Classic.Thing
  pipelinePath: foo
  purpose: stuff`,
			err: false,
		},
		{
			name: "duplicate service group",
			input: `entrypoints:
- identifier: Microsoft.Azure.ARO.HCP.Whatever.Child
services:
- serviceGroup: Microsoft.Azure.ARO.HCP
  pipelinePath: foo
  purpose: stuff
  children:
  - serviceGroup: Microsoft.Azure.ARO.HCP.Whatever
    pipelinePath: foo
    purpose: stuff
    children:
    - serviceGroup: Microsoft.Azure.ARO.HCP.Whatever.Child
      pipelinePath: foo
      purpose: stuff
  - serviceGroup: Microsoft.Azure.ARO.HCP.Whatever
    pipelinePath: foo
    purpose: stuff
- serviceGroup: Microsoft.Azure.ARO.HCP.Classic.Thing
  pipelinePath: foo
  purpose: stuff`,
			err: true,
		},
		{
			name: "missing entrypoint",
			input: `entrypoints:
- identifier: Microsoft.Azure.ARO.HCP.Whatever.Missing
services:
- serviceGroup: Microsoft.Azure.ARO.HCP
  pipelinePath: foo
  purpose: stuff
  children:
  - serviceGroup: Microsoft.Azure.ARO.HCP.Whatever
    pipelinePath: foo
    purpose: stuff
    children:
    - serviceGroup: Microsoft.Azure.ARO.HCP.Whatever.Child
      pipelinePath: foo
      purpose: stuff
  - serviceGroup: Microsoft.Azure.ARO.HCP.Other
    pipelinePath: foo
    purpose: stuff
- serviceGroup: Microsoft.Azure.ARO.HCP.Classic.Thing
  pipelinePath: foo
  purpose: stuff`,
			err: true,
		},
		{
			name: "empty identifier",
			input: `entrypoints:
- identifier: ''
services:
- serviceGroup: Microsoft.Azure.ARO.HCP
  pipelinePath: foo
  purpose: stuff
  children:
  - serviceGroup: Microsoft.Azure.ARO.HCP.Whatever
    pipelinePath: foo
    purpose: stuff
    children:
    - serviceGroup: Microsoft.Azure.ARO.HCP.Whatever.Child
      pipelinePath: foo
      purpose: stuff
  - serviceGroup: Microsoft.Azure.ARO.HCP.Other
    pipelinePath: foo
    purpose: stuff
- serviceGroup: Microsoft.Azure.ARO.Classic.Thing
  pipelinePath: foo
  purpose: stuff`,
			err: true,
		},
		{
			name: "empty purpose, no metadata",
			input: `services:
- serviceGroup: Microsoft.Azure
  pipelinePath: foo`,
			err: true,
		},
		{
			name: "empty purpose, empty key in metadata",
			input: `services:
- serviceGroup: Microsoft.Azure.ARO.HCP.Whatever
  pipelinePath: foo
  metadata:
    purpose: ''`,
			err: true,
		},
		{
			name: "empty purpose, defaults from metadata",
			input: `services:
- serviceGroup: Microsoft.Azure.ARO.HCP.Whatever
  pipelinePath: foo
  metadata:
    purpose: stuff`,
			err: false,
		},
		{
			name: "empty pipeline, no metadata",
			input: `services:
- serviceGroup: Microsoft.Azure.ARO.HCP.Whatever
  purpose: stuff`,
			err: true,
		},
		{
			name: "empty pipeline, empty key in metadata",
			input: `services:
- serviceGroup: Microsoft.Azure.ARO.HCP.Whatever
  purpose: stuff
  metadata:
    pipeline: ''`,
			err: true,
		},
		{
			name: "empty pipeline, defaults from metadata",
			input: `services:
- serviceGroup: Microsoft.Azure.ARO.HCP.Whatever
  purpose: stuff
  metadata:
    pipeline: foo`,
			err: false,
		},
		{
			name: "invalid service group prefix",
			input: `services:
- serviceGroup: Microsoft.Azure.ContainerService.Something.Other
  purpose: stuff
  metadata:
    pipeline: foo`,
			err: true,
		},
		{
			name: "ARO.Classic service group component",
			input: `services:
- serviceGroup: Microsoft.Azure.ARO.Classic.Whatever
  purpose: stuff
  metadata:
    pipeline: foo`,
			err: false,
		},
		{
			name: "ARO.ARMManifest service group component",
			input: `services:
- serviceGroup: Microsoft.Azure.ARO.ARMManifest.Whatever
  purpose: stuff
  metadata:
    pipeline: foo`,
			err: false,
		},
		{
			name: "unknown service group component",
			input: `services:
- serviceGroup: Microsoft.Azure.ARO.Oops.Doops.Troops
  purpose: stuff
  metadata:
    pipeline: foo`,
			err: false,
		},
		{
			name: "missing service group component",
			input: `services:
- serviceGroup: Microsoft.Azure.ARO.
  purpose: stuff
  metadata:
    pipeline: foo`,
			err: true,
		},
		{
			name: "too many nested service group components",
			input: `services:
- serviceGroup: Microsoft.Azure.ARO.HCP.Whatever.Child.Thing
  purpose: stuff
  metadata:
    pipeline: foo`,
			err: true,
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			var topo Topology
			if err := yaml.Unmarshal([]byte(testCase.input), &topo); err != nil {
				t.Fatalf("failed to unmarshal: %s", err)
			}
			err := topo.Validate()
			if err == nil && testCase.err {
				t.Errorf("expected error, got none")
			}
			if err != nil && !testCase.err {
				t.Errorf("expected no error, got: %v", err)
			}
		})
	}
}

func TestDependency_Lookup(t *testing.T) {
	for _, testCase := range []struct {
		name       string
		input      string
		identifier string
		expected   *Service
		err        bool
		notFound   bool
	}{
		{
			name: "missing",
			input: `services:
- serviceGroup: Microsoft.Azure
  children:
  - serviceGroup: Microsoft.Azure.ARO
    children:
    - serviceGroup: Microsoft.Azure.ARO.HCP
  - serviceGroup: Microsoft.Azure.C
- serviceGroup: A.B`,
			identifier: "1.2.3",
			err:        true,
			notFound:   true,
		},
		{
			name: "top-level",
			input: `services:
- serviceGroup: Microsoft.Azure
  children:
  - serviceGroup: Microsoft.Azure.ARO
    children:
    - serviceGroup: Microsoft.Azure.ARO.HCP
  - serviceGroup: Microsoft.Azure.C
- serviceGroup: A.B`,
			identifier: "Microsoft.Azure",
			expected: &Service{
				ServiceGroup: "Microsoft.Azure",
				Children: []Service{
					{
						ServiceGroup: "Microsoft.Azure.ARO",
						Children: []Service{
							{
								ServiceGroup: "Microsoft.Azure.ARO.HCP",
							},
						},
					},
					{
						ServiceGroup: "Microsoft.Azure.C",
					},
				},
			},
		},
		{
			name: "mid-level",
			input: `services:
- serviceGroup: Microsoft.Azure
  children:
  - serviceGroup: Microsoft.Azure.ARO
    children:
    - serviceGroup: Microsoft.Azure.ARO.HCP
  - serviceGroup: Microsoft.Azure.C
- serviceGroup: A.B`,
			identifier: "Microsoft.Azure.ARO",
			expected: &Service{
				ServiceGroup: "Microsoft.Azure.ARO",
				Children: []Service{
					{
						ServiceGroup: "Microsoft.Azure.ARO.HCP",
					},
				},
			},
		},
		{
			name: "leaf",
			input: `services:
- serviceGroup: Microsoft.Azure
  children:
  - serviceGroup: Microsoft.Azure.ARO
    children:
    - serviceGroup: Microsoft.Azure.ARO.HCP
  - serviceGroup: Microsoft.Azure.C
- serviceGroup: A.B`,
			identifier: "Microsoft.Azure.ARO.HCP",
			expected: &Service{
				ServiceGroup: "Microsoft.Azure.ARO.HCP",
			},
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			var topo Topology
			if err := yaml.Unmarshal([]byte(testCase.input), &topo); err != nil {
				t.Fatalf("failed to unmarshal: %s", err)
			}
			build, err := topo.Lookup(testCase.identifier)
			if err == nil && testCase.err {
				t.Errorf("expected error, got none")
			}
			if err != nil && !testCase.err {
				t.Errorf("expected no error, got: %v", err)
			}
			if diff := cmp.Diff(build, testCase.expected); diff != "" {
				t.Errorf("build: (-want got):\n%s", diff)
			}
		})
	}
}

func TestPropagateStamped(t *testing.T) {
	for _, testCase := range []struct {
		name     string
		input    Topology
		expected Topology
	}{
		{
			name: "parent stamped, children inherit",
			input: Topology{
				Services: []Service{
					{
						ServiceGroup: "parent",
						Stamped:      true,
						Children: []Service{
							{ServiceGroup: "child-a"},
							{ServiceGroup: "child-b", Children: []Service{
								{ServiceGroup: "grandchild"},
							}},
						},
					},
				},
			},
			expected: Topology{
				Services: []Service{
					{
						ServiceGroup: "parent",
						Stamped:      true,
						Children: []Service{
							{ServiceGroup: "child-a", Stamped: true},
							{ServiceGroup: "child-b", Stamped: true, Children: []Service{
								{ServiceGroup: "grandchild", Stamped: true},
							}},
						},
					},
				},
			},
		},
		{
			name: "siblings unaffected",
			input: Topology{
				Services: []Service{
					{
						ServiceGroup: "stamped-parent",
						Stamped:      true,
						Children: []Service{
							{ServiceGroup: "stamped-child"},
						},
					},
					{
						ServiceGroup: "non-stamped-sibling",
						Children: []Service{
							{ServiceGroup: "non-stamped-child"},
						},
					},
				},
			},
			expected: Topology{
				Services: []Service{
					{
						ServiceGroup: "stamped-parent",
						Stamped:      true,
						Children: []Service{
							{ServiceGroup: "stamped-child", Stamped: true},
						},
					},
					{
						ServiceGroup: "non-stamped-sibling",
						Children: []Service{
							{ServiceGroup: "non-stamped-child"},
						},
					},
				},
			},
		},
		{
			name: "no stamped services, no changes",
			input: Topology{
				Services: []Service{
					{
						ServiceGroup: "parent",
						Children: []Service{
							{ServiceGroup: "child"},
						},
					},
				},
			},
			expected: Topology{
				Services: []Service{
					{
						ServiceGroup: "parent",
						Children: []Service{
							{ServiceGroup: "child"},
						},
					},
				},
			},
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			testCase.input.PropagateStamped()
			if diff := cmp.Diff(testCase.expected, testCase.input); diff != "" {
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestLoadCombined_TopologyDirMapping(t *testing.T) {
	teamAPath := filepath.Join("testdata", "team-a", "topology.yaml")
	teamBPath := filepath.Join("testdata", "team-b", "topology.yaml")

	got, err := LoadCombined([]string{teamAPath, teamBPath})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	wantDirMap := map[string]string{
		"Microsoft.Azure.ARO.HCP":           filepath.Join("testdata", "team-a"),
		"Microsoft.Azure.ARO.HCP.Child":     filepath.Join("testdata", "team-a"),
		"Microsoft.Azure.ARO.Classic":       filepath.Join("testdata", "team-b"),
		"Microsoft.Azure.ARO.HCP.Extension": filepath.Join("testdata", "team-b"),
	}

	for serviceGroup, wantDir := range wantDirMap {
		gotDir, err := got.GetTopologyDirForServiceGroup(serviceGroup)
		if err != nil {
			t.Errorf("unexpected error for %s: %v", serviceGroup, err)
			continue
		}
		if gotDir != wantDir {
			t.Errorf("for service group %s: want dir %q, got %q", serviceGroup, wantDir, gotDir)
		}
	}
}

func TestLoadCombined(t *testing.T) {
	externalParentHCP := "Microsoft.Azure.ARO.HCP"
	externalParentOther := "Microsoft.Azure.ARO.Other"

	writeTempTopology := func(t *testing.T, content string) string {
		t.Helper()
		f, err := os.CreateTemp("", "topology-*.yaml")
		if err != nil {
			t.Fatalf("failed to create temp file: %s", err)
		}
		t.Cleanup(func() {
			if err := os.Remove(f.Name()); err != nil {
				t.Errorf("failed to remove temp file: %v", err)
			}
		})
		if _, err := f.WriteString(content); err != nil {
			t.Fatalf("failed to write topology: %s", err)
		}
		if err := f.Close(); err != nil {
			t.Fatalf("failed to close topology file: %s", err)
		}
		return f.Name()
	}

	for _, testCase := range []struct {
		name       string
		topologies []string
		expected   *Topology
		err        bool
	}{
		{
			name: "single topology",
			topologies: []string{`services:
- serviceGroup: Microsoft.Azure.ARO.HCP
  pipelinePath: foo
  purpose: stuff`},
			expected: &Topology{
				Services: []Service{
					{
						ServiceGroup: "Microsoft.Azure.ARO.HCP",
						PipelinePath: "foo",
						Purpose:      "stuff",
					},
				},
			},
		},
		{
			name: "two topologies with cross-dependencies",
			topologies: []string{
				`services:
- serviceGroup: Microsoft.Azure.ARO.HCP
  pipelinePath: foo
  purpose: stuff
- serviceGroup: Microsoft.Azure.ARO.Classic.Child
  pipelinePath: baz
  purpose: classic child
  externalParent: Microsoft.Azure.ARO.Other`,
				`services:
- serviceGroup: Microsoft.Azure.ARO.Other
  pipelinePath: bar
  purpose: other stuff
- serviceGroup: Microsoft.Azure.ARO.HCP.Child
  pipelinePath: qux
  purpose: hcp child
  externalParent: Microsoft.Azure.ARO.HCP`,
			},
			expected: &Topology{
				Services: []Service{
					{
						ServiceGroup: "Microsoft.Azure.ARO.HCP",
						PipelinePath: "foo",
						Purpose:      "stuff",
						Children: []Service{
							{
								ServiceGroup:   "Microsoft.Azure.ARO.HCP.Child",
								PipelinePath:   "qux",
								Purpose:        "hcp child",
								ExternalParent: &externalParentHCP,
							},
						},
					},
					{
						ServiceGroup: "Microsoft.Azure.ARO.Other",
						PipelinePath: "bar",
						Purpose:      "other stuff",
						Children: []Service{
							{
								ServiceGroup:   "Microsoft.Azure.ARO.Classic.Child",
								PipelinePath:   "baz",
								Purpose:        "classic child",
								ExternalParent: &externalParentOther,
							},
						},
					},
				},
			},
		},
		{
			name: "single topology with externalParent is not an error",
			topologies: []string{`services:
- serviceGroup: Microsoft.Azure.ARO.HCP
  pipelinePath: foo
  purpose: stuff
- serviceGroup: Microsoft.Azure.ARO.HCP.Child
  pipelinePath: bar
  purpose: child
  externalParent: Microsoft.Azure.ARO.HCP`},
			expected: &Topology{
				Services: []Service{
					{
						ServiceGroup: "Microsoft.Azure.ARO.HCP",
						PipelinePath: "foo",
						Purpose:      "stuff",
					},
					{
						ServiceGroup:   "Microsoft.Azure.ARO.HCP.Child",
						PipelinePath:   "bar",
						Purpose:        "child",
						ExternalParent: func() *string { s := "Microsoft.Azure.ARO.HCP"; return &s }(),
					},
				},
			},
		},
		{
			name: "chained external parents across three topologies in reverse order",
			topologies: []string{
				// topology 3: C depends on B
				`services:
- serviceGroup: Microsoft.Azure.ARO.HCP.B.C
  pipelinePath: c.yaml
  purpose: service c
  externalParent: Microsoft.Azure.ARO.HCP.B`,
				// topology 2: B depends on A
				`services:
- serviceGroup: Microsoft.Azure.ARO.HCP.B
  pipelinePath: b.yaml
  purpose: service b
  externalParent: Microsoft.Azure.ARO.HCP`,
				// topology 1: A is the root
				`services:
- serviceGroup: Microsoft.Azure.ARO.HCP
  pipelinePath: a.yaml
  purpose: service a`,
			},
			expected: &Topology{
				Services: []Service{
					{
						ServiceGroup: "Microsoft.Azure.ARO.HCP",
						PipelinePath: "a.yaml",
						Purpose:      "service a",
						Children: []Service{
							{
								ServiceGroup:   "Microsoft.Azure.ARO.HCP.B",
								PipelinePath:   "b.yaml",
								Purpose:        "service b",
								ExternalParent: func() *string { s := "Microsoft.Azure.ARO.HCP"; return &s }(),
								Children: []Service{
									{
										ServiceGroup:   "Microsoft.Azure.ARO.HCP.B.C",
										PipelinePath:   "c.yaml",
										Purpose:        "service c",
										ExternalParent: func() *string { s := "Microsoft.Azure.ARO.HCP.B"; return &s }(),
									},
								},
							},
						},
					},
				},
			},
		},
		{
			name: "errors when a nested child has externalParent set",
			topologies: []string{`services:
- serviceGroup: Microsoft.Azure.ARO.HCP
  pipelinePath: foo
  purpose: stuff
- serviceGroup: Microsoft.Azure.ARO.HCP.Parent
  pipelinePath: bar
  purpose: parent
  children:
  - serviceGroup: Microsoft.Azure.ARO.HCP.Parent.Child
    pipelinePath: baz
    purpose: child
    externalParent: Microsoft.Azure.ARO.HCP`},
			err: true,
		},
		{
			name: "errors when external parent does not exist",
			topologies: []string{
				`services:
- serviceGroup: Microsoft.Azure.ARO.HCP
  pipelinePath: foo
  purpose: stuff`,
				`services:
- serviceGroup: Microsoft.Azure.ARO.HCP.Child
  pipelinePath: bar
  purpose: child
  externalParent: Microsoft.Azure.ARO.DoesNotExist`,
			},
			err: true,
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			paths := make([]string, len(testCase.topologies))
			for i, content := range testCase.topologies {
				paths[i] = writeTempTopology(t, content)
			}

			got, err := LoadCombined(paths)
			if err == nil && testCase.err {
				t.Errorf("expected error, got none")
			}
			if err != nil && !testCase.err {
				t.Fatalf("expected no error, got: %v", err)
			}
			if testCase.expected != nil {
				if diff := cmp.Diff(testCase.expected, &got.Topology); diff != "" {
					t.Errorf("combined topology mismatch (-want +got):\n%s", diff)
				}
			}
		})
	}
}
