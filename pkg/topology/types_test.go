package topology

import (
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
