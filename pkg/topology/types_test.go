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
- identifier: a.b.c
services:
- serviceGroup: a.b
  children:
  - serviceGroup: a.b.c
    children:
    - serviceGroup: a.b.c.d
  - serviceGroup: a.b.C
- serviceGroup: A.B`,
			err: false,
		},
		{
			name: "duplicate service group",
			input: `entrypoints:
- identifier: a.b.c
services:
- serviceGroup: a.b
  children:
  - serviceGroup: a.b.c
    children:
    - serviceGroup: a.b.c.d
  - serviceGroup: a.b.c.d
- serviceGroup: A.B`,
			err: true,
		},
		{
			name: "missing entrypoint",
			input: `entrypoints:
- identifier: a.b.c.d.e
services:
- serviceGroup: a.b
  children:
  - serviceGroup: a.b.c
    children:
    - serviceGroup: a.b.c.d
  - serviceGroup: a.b.c.d
- serviceGroup: A.B`,
			err: true,
		},
		{
			name: "empty identifier",
			input: `entrypoints:
- identifier: ''
services:
- serviceGroup: a.b
  children:
  - serviceGroup: a.b.c
    children:
    - serviceGroup: a.b.c.d
  - serviceGroup: a.b.c.d
- serviceGroup: A.B`,
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
- serviceGroup: a.b
  children:
  - serviceGroup: a.b.c
    children:
    - serviceGroup: a.b.c.d
  - serviceGroup: a.b.C
- serviceGroup: A.B`,
			identifier: "1.2.3",
			err:        true,
			notFound:   true,
		},
		{
			name: "top-level",
			input: `services:
- serviceGroup: a.b
  children:
  - serviceGroup: a.b.c
    children:
    - serviceGroup: a.b.c.d
  - serviceGroup: a.b.C
- serviceGroup: A.B`,
			identifier: "a.b",
			expected: &Service{
				ServiceGroup: "a.b",
				Children: []Service{
					{
						ServiceGroup: "a.b.c",
						Children: []Service{
							{
								ServiceGroup: "a.b.c.d",
							},
						},
					},
					{
						ServiceGroup: "a.b.C",
					},
				},
			},
		},
		{
			name: "mid-level",
			input: `services:
- serviceGroup: a.b
  children:
  - serviceGroup: a.b.c
    children:
    - serviceGroup: a.b.c.d
  - serviceGroup: a.b.C
- serviceGroup: A.B`,
			identifier: "a.b.c",
			expected: &Service{
				ServiceGroup: "a.b.c",
				Children: []Service{
					{
						ServiceGroup: "a.b.c.d",
					},
				},
			},
		},
		{
			name: "leaf",
			input: `services:
- serviceGroup: a.b
  children:
  - serviceGroup: a.b.c
    children:
    - serviceGroup: a.b.c.d
  - serviceGroup: a.b.C
- serviceGroup: A.B`,
			identifier: "a.b.c.d",
			expected: &Service{
				ServiceGroup: "a.b.c.d",
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
