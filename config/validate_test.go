// Copyright 2025 Microsoft Corporation
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package config_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Azure/ARO-Tools/config"
)

func TestValidateSimpleFieldAccess(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		template   string
		wantErrMsg string
	}{
		{
			name:     "simple field access",
			template: "{{ .foo }}",
		},
		{
			name:     "nested field access",
			template: "{{ .foo.bar.baz }}",
		},
		{
			name:     "plain text only",
			template: "no templates here",
		},
		{
			name:     "empty string",
			template: "",
		},
		{
			name:     "mixed text and fields",
			template: "prefix {{ .x }} middle {{ .y }} suffix",
		},
		{
			name:     "whitespace trimming",
			template: "{{- .foo -}}",
		},
		{
			name:     "multiple fields on separate lines",
			template: "{{ .a }}\n{{ .b }}\n{{ .c }}",
		},
		{
			name:     "range loop",
			template: "{{ range .items }}{{ .name }}{{ end }}",
			wantErrMsg: "template contains restricted constructs:\n" +
				"  line 1: range loop not allowed",
		},
		{
			name:     "if conditional",
			template: "{{ if .x }}yes{{ end }}",
			wantErrMsg: "template contains restricted constructs:\n" +
				"  line 1: if conditional not allowed",
		},
		{
			name:     "if else",
			template: "{{ if .x }}a{{ else }}b{{ end }}",
			wantErrMsg: "template contains restricted constructs:\n" +
				"  line 1: if conditional not allowed",
		},
		{
			name:     "with block",
			template: "{{ with .x }}{{ .y }}{{ end }}",
			wantErrMsg: "template contains restricted constructs:\n" +
				"  line 1: with block not allowed",
		},
		{
			name:     "variable declaration",
			template: "{{ $x := .foo }}",
			wantErrMsg: "template contains restricted constructs:\n" +
				"  line 1: variable declaration $x not allowed",
		},
		{
			name:     "function call printf",
			template: `{{ printf "%s" .x }}`,
			wantErrMsg: "template contains restricted constructs:\n" +
				`  line 1: function call "printf" not allowed`,
		},
		{
			name:     "builtin function eq",
			template: `{{ eq .x "foo" }}`,
			wantErrMsg: "template contains restricted constructs:\n" +
				`  line 1: function call "eq" not allowed`,
		},
		{
			name:     "pipe operator",
			template: "{{ .x | len }}",
			wantErrMsg: "template contains restricted constructs:\n" +
				"  line 1: pipe not allowed",
		},
		{
			name:     "template invocation",
			template: `{{ template "sub" }}`,
			wantErrMsg: "template contains restricted constructs:\n" +
				"  line 1: template invocation not allowed",
		},
		{
			name:     "dot only access",
			template: "{{ . }}",
			wantErrMsg: "template contains restricted constructs:\n" +
				"  line 1: dot-only access not allowed, use a named field",
		},
		{
			name:     "variable reference",
			template: "{{ $x := .foo }}{{ $x }}",
			wantErrMsg: "template contains restricted constructs:\n" +
				"  line 1: variable declaration $x not allowed\n" +
				`  line 1: variable reference "$x" not allowed`,
		},
		{
			name:     "multiple violations collected",
			template: "{{ range .items }}{{ if .ok }}{{ .name }}{{ end }}{{ end }}",
			wantErrMsg: "template contains restricted constructs:\n" +
				"  line 1: range loop not allowed\n" +
				"  line 1: if conditional not allowed",
		},
		{
			name:     "line numbers reported",
			template: "{{ .ok }}\n{{ .also_ok }}\n{{ range .items }}{{ end }}",
			wantErrMsg: "template contains restricted constructs:\n" +
				"  line 3: range loop not allowed",
		},
		{
			name:       "undefined function parse error",
			template:   "{{ myFunc .x }}",
			wantErrMsg: `failed to parse template: template: :1: function "myFunc" not defined`,
		},
		{
			name:       "malformed template syntax",
			template:   "{{ .foo }",
			wantErrMsg: `failed to parse template: template: :1: unexpected "}" in operand`,
		},
		{
			name:     "string literal",
			template: `{{ "hello" }}`,
			wantErrMsg: "template contains restricted constructs:\n" +
				"  line 1: literal value not allowed, only field access ({{ .field }}) permitted",
		},
		{
			name:     "number literal",
			template: "{{ 42 }}",
			wantErrMsg: "template contains restricted constructs:\n" +
				"  line 1: literal value not allowed, only field access ({{ .field }}) permitted",
		},
		{
			name:     "bool literal",
			template: "{{ true }}",
			wantErrMsg: "template contains restricted constructs:\n" +
				"  line 1: literal value not allowed, only field access ({{ .field }}) permitted",
		},
		{
			name:     "nil literal",
			template: "{{ nil }}",
			wantErrMsg: "template contains restricted constructs:\n" +
				"  line 1: literal value not allowed, only field access ({{ .field }}) permitted",
		},
		{
			name:     "block directive",
			template: `{{ block "name" . }}content{{ end }}`,
			wantErrMsg: "template contains restricted constructs:\n" +
				"  line 1: template invocation not allowed",
		},
		{
			name:     "comment is allowed",
			template: "{{/* this is a comment */}}{{ .foo }}",
		},
		{
			name:     "nested template define",
			template: `{{ define "helper" }}{{ .x }}{{ end }}{{ template "helper" }}`,
			wantErrMsg: "template contains restricted constructs:\n" +
				"  line 1: template invocation not allowed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := config.ValidateSimpleFieldAccess([]byte(tt.template))
			if len(tt.wantErrMsg) != 0 {
				require.EqualError(t, err, tt.wantErrMsg)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
