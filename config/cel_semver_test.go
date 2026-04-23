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

package config

import (
	"testing"

	"github.com/google/cel-go/common/types"
	"github.com/stretchr/testify/require"
)

func TestParseSemver(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		input     string
		wantRaw   string
		wantMajor int
		wantMinor int
		wantPatch int
		wantErr   string
	}{
		{
			name:      "standard three-part",
			input:     "1.2.3",
			wantRaw:   "v1.2.3",
			wantMajor: 1,
			wantMinor: 2,
			wantPatch: 3,
		},
		{
			name:      "with v prefix",
			input:     "v1.2.3",
			wantRaw:   "v1.2.3",
			wantMajor: 1,
			wantMinor: 2,
			wantPatch: 3,
		},
		{
			name:      "two-part canonicalized",
			input:     "1.0",
			wantRaw:   "v1.0.0",
			wantMajor: 1,
			wantMinor: 0,
			wantPatch: 0,
		},
		{
			name:      "with prerelease",
			input:     "1.2.3-alpha.1",
			wantRaw:   "v1.2.3-alpha.1",
			wantMajor: 1,
			wantMinor: 2,
			wantPatch: 3,
		},
		{
			name:      "with build metadata",
			input:     "1.2.3+build.42",
			wantRaw:   "v1.2.3",
			wantMajor: 1,
			wantMinor: 2,
			wantPatch: 3,
		},
		{
			name:    "empty string",
			input:   "",
			wantErr: "invalid semver",
		},
		{
			name:    "garbage",
			input:   "not-a-version",
			wantErr: "invalid semver",
		},
		{
			name:    "double v prefix",
			input:   "vv1.2.3",
			wantErr: "invalid semver",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			sv, err := parseSemver(tt.input)
			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.wantRaw, sv.raw)
			require.Equal(t, tt.wantMajor, sv.major)
			require.Equal(t, tt.wantMinor, sv.minor)
			require.Equal(t, tt.wantPatch, sv.patch)
		})
	}
}

func TestCELSemver(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		schema    string
		value     map[string]any
		wantError string
	}{
		{
			name: "isSemver passes for valid semver",
			schema: `{
				"type": "object",
				"properties": {"version": {"type": "string"}},
				"x-cel-validations": [
					{"rule": "isSemver(self.version)", "message": "must be valid semver"}
				]
			}`,
			value: map[string]any{"version": "1.2.3"},
		},
		{
			name: "isSemver passes with v prefix",
			schema: `{
				"type": "object",
				"properties": {"version": {"type": "string"}},
				"x-cel-validations": [
					{"rule": "isSemver(self.version)", "message": "must be valid semver"}
				]
			}`,
			value: map[string]any{"version": "v1.2.3"},
		},
		{
			name: "isSemver fails for invalid",
			schema: `{
				"type": "object",
				"properties": {"version": {"type": "string"}},
				"x-cel-validations": [
					{"rule": "isSemver(self.version)", "message": "must be valid semver"}
				]
			}`,
			value:     map[string]any{"version": "not-a-version"},
			wantError: "must be valid semver",
		},
		{
			name: "semver().major()",
			schema: `{
				"type": "object",
				"properties": {"version": {"type": "string"}},
				"x-cel-validations": [
					{"rule": "semver(self.version).major() >= 1", "message": "must be at least v1"}
				]
			}`,
			value: map[string]any{"version": "2.0.0"},
		},
		{
			name: "semver().major() fails",
			schema: `{
				"type": "object",
				"properties": {"version": {"type": "string"}},
				"x-cel-validations": [
					{"rule": "semver(self.version).major() >= 1", "message": "must be at least v1"}
				]
			}`,
			value:     map[string]any{"version": "0.9.0"},
			wantError: "must be at least v1",
		},
		{
			name: "semver().minor()",
			schema: `{
				"type": "object",
				"properties": {"version": {"type": "string"}},
				"x-cel-validations": [
					{"rule": "semver(self.version).minor() == 2", "message": "minor must be 2"}
				]
			}`,
			value: map[string]any{"version": "1.2.3"},
		},
		{
			name: "semver().patch()",
			schema: `{
				"type": "object",
				"properties": {"version": {"type": "string"}},
				"x-cel-validations": [
					{"rule": "semver(self.version).patch() == 3", "message": "patch must be 3"}
				]
			}`,
			value: map[string]any{"version": "1.2.3"},
		},
		{
			name: "semver().compareTo() greater",
			schema: `{
				"type": "object",
				"properties": {"current": {"type": "string"}, "minimum": {"type": "string"}},
				"x-cel-validations": [
					{"rule": "semver(self.current).compareTo(semver(self.minimum)) >= 0", "message": "current must be >= minimum"}
				]
			}`,
			value: map[string]any{"current": "2.0.0", "minimum": "1.5.0"},
		},
		{
			name: "semver().compareTo() fails when less",
			schema: `{
				"type": "object",
				"properties": {"current": {"type": "string"}, "minimum": {"type": "string"}},
				"x-cel-validations": [
					{"rule": "semver(self.current).compareTo(semver(self.minimum)) >= 0", "message": "current must be >= minimum"}
				]
			}`,
			value:     map[string]any{"current": "1.0.0", "minimum": "1.5.0"},
			wantError: "current must be >= minimum",
		},
		{
			name: "semver with prerelease",
			schema: `{
				"type": "object",
				"properties": {"version": {"type": "string"}},
				"x-cel-validations": [
					{"rule": "isSemver(self.version)", "message": "must be valid semver"}
				]
			}`,
			value: map[string]any{"version": "1.2.3-alpha.1"},
		},
		{
			name: "isSemver passes for two-part version (canonicalized to x.y.0)",
			schema: `{
				"type": "object",
				"properties": {"version": {"type": "string"}},
				"x-cel-validations": [
					{"rule": "isSemver(self.version)", "message": "must be valid semver"}
				]
			}`,
			value: map[string]any{"version": "1.0"},
		},
		{
			name: "semver().compareTo() equal",
			schema: `{
				"type": "object",
				"properties": {"a": {"type": "string"}, "b": {"type": "string"}},
				"x-cel-validations": [
					{"rule": "semver(self.a).compareTo(semver(self.b)) == 0", "message": "versions must be equal"}
				]
			}`,
			value: map[string]any{"a": "1.2.3", "b": "1.2.3"},
		},
		{
			name: "semver().compareTo() equal fails for different versions",
			schema: `{
				"type": "object",
				"properties": {"a": {"type": "string"}, "b": {"type": "string"}},
				"x-cel-validations": [
					{"rule": "semver(self.a).compareTo(semver(self.b)) == 0", "message": "versions must be equal"}
				]
			}`,
			value:     map[string]any{"a": "1.2.3", "b": "1.2.4"},
			wantError: "versions must be equal",
		},
		{
			name: "semver > semver passes",
			schema: `{
				"type": "object",
				"properties": {"a": {"type": "string"}, "b": {"type": "string"}},
				"x-cel-validations": [
					{"rule": "semver(self.a) > semver(self.b)", "message": "a must be greater than b"}
				]
			}`,
			value: map[string]any{"a": "2.0.0", "b": "1.0.0"},
		},
		{
			name: "semver > semver fails",
			schema: `{
				"type": "object",
				"properties": {"a": {"type": "string"}, "b": {"type": "string"}},
				"x-cel-validations": [
					{"rule": "semver(self.a) > semver(self.b)", "message": "a must be greater than b"}
				]
			}`,
			value:     map[string]any{"a": "1.0.0", "b": "2.0.0"},
			wantError: "a must be greater than b",
		},
		{
			name: "semver > semver fails for equal",
			schema: `{
				"type": "object",
				"properties": {"a": {"type": "string"}, "b": {"type": "string"}},
				"x-cel-validations": [
					{"rule": "semver(self.a) > semver(self.b)", "message": "a must be greater than b"}
				]
			}`,
			value:     map[string]any{"a": "1.0.0", "b": "1.0.0"},
			wantError: "a must be greater than b",
		},
		{
			name: "semver >= semver passes for equal",
			schema: `{
				"type": "object",
				"properties": {"a": {"type": "string"}, "b": {"type": "string"}},
				"x-cel-validations": [
					{"rule": "semver(self.a) >= semver(self.b)", "message": "a must be >= b"}
				]
			}`,
			value: map[string]any{"a": "1.0.0", "b": "1.0.0"},
		},
		{
			name: "semver >= semver fails when strictly less",
			schema: `{
				"type": "object",
				"properties": {"a": {"type": "string"}, "b": {"type": "string"}},
				"x-cel-validations": [
					{"rule": "semver(self.a) >= semver(self.b)", "message": "a must be >= b"}
				]
			}`,
			value:     map[string]any{"a": "1.0.0", "b": "2.0.0"},
			wantError: "a must be >= b",
		},
		{
			name: "semver < semver passes",
			schema: `{
				"type": "object",
				"properties": {"a": {"type": "string"}, "b": {"type": "string"}},
				"x-cel-validations": [
					{"rule": "semver(self.a) < semver(self.b)", "message": "a must be less than b"}
				]
			}`,
			value: map[string]any{"a": "1.0.0", "b": "2.0.0"},
		},
		{
			name: "semver < semver fails",
			schema: `{
				"type": "object",
				"properties": {"a": {"type": "string"}, "b": {"type": "string"}},
				"x-cel-validations": [
					{"rule": "semver(self.a) < semver(self.b)", "message": "a must be less than b"}
				]
			}`,
			value:     map[string]any{"a": "2.0.0", "b": "1.0.0"},
			wantError: "a must be less than b",
		},
		{
			name: "semver < semver fails for equal",
			schema: `{
				"type": "object",
				"properties": {"a": {"type": "string"}, "b": {"type": "string"}},
				"x-cel-validations": [
					{"rule": "semver(self.a) < semver(self.b)", "message": "a must be less than b"}
				]
			}`,
			value:     map[string]any{"a": "1.0.0", "b": "1.0.0"},
			wantError: "a must be less than b",
		},
		{
			name: "semver <= semver passes for equal",
			schema: `{
				"type": "object",
				"properties": {"a": {"type": "string"}, "b": {"type": "string"}},
				"x-cel-validations": [
					{"rule": "semver(self.a) <= semver(self.b)", "message": "a must be <= b"}
				]
			}`,
			value: map[string]any{"a": "1.0.0", "b": "1.0.0"},
		},
		{
			name: "semver <= semver fails when strictly greater",
			schema: `{
				"type": "object",
				"properties": {"a": {"type": "string"}, "b": {"type": "string"}},
				"x-cel-validations": [
					{"rule": "semver(self.a) <= semver(self.b)", "message": "a must be <= b"}
				]
			}`,
			value:     map[string]any{"a": "2.0.0", "b": "1.0.0"},
			wantError: "a must be <= b",
		},
		{
			name: "semver().minor() fails",
			schema: `{
				"type": "object",
				"properties": {"version": {"type": "string"}},
				"x-cel-validations": [
					{"rule": "semver(self.version).minor() == 2", "message": "minor must be 2"}
				]
			}`,
			value:     map[string]any{"version": "1.0.0"},
			wantError: "minor must be 2",
		},
		{
			name: "semver().patch() fails",
			schema: `{
				"type": "object",
				"properties": {"version": {"type": "string"}},
				"x-cel-validations": [
					{"rule": "semver(self.version).patch() == 3", "message": "patch must be 3"}
				]
			}`,
			value:     map[string]any{"version": "1.2.0"},
			wantError: "patch must be 3",
		},
		{
			name: "semver equality passes for same version",
			schema: `{
				"type": "object",
				"properties": {"a": {"type": "string"}, "b": {"type": "string"}},
				"x-cel-validations": [
					{"rule": "semver(self.a) == semver(self.b)", "message": "versions must be equal"}
				]
			}`,
			value: map[string]any{"a": "1.2.3", "b": "1.2.3"},
		},
		{
			name: "semver equality fails for different versions",
			schema: `{
				"type": "object",
				"properties": {"a": {"type": "string"}, "b": {"type": "string"}},
				"x-cel-validations": [
					{"rule": "semver(self.a) == semver(self.b)", "message": "versions must be equal"}
				]
			}`,
			value:     map[string]any{"a": "1.2.3", "b": "1.2.4"},
			wantError: "versions must be equal",
		},
		{
			name: "prerelease is less than release",
			schema: `{
				"type": "object",
				"properties": {"a": {"type": "string"}, "b": {"type": "string"}},
				"x-cel-validations": [
					{"rule": "semver(self.a) < semver(self.b)", "message": "a must be less than b"}
				]
			}`,
			value: map[string]any{"a": "1.2.3-alpha.1", "b": "1.2.3"},
		},
		{
			name: "semver() constructor error for invalid input",
			schema: `{
				"type": "object",
				"properties": {"version": {"type": "string"}},
				"x-cel-validations": [
					{"rule": "semver(self.version).major() >= 1", "message": "must be at least v1"}
				]
			}`,
			value:     map[string]any{"version": "not-a-version"},
			wantError: "CEL evaluation error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			sch := compileWithCEL(t, tt.schema)
			err := sch.Validate(tt.value)
			if tt.wantError == "" {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
				require.Contains(t, err.Error(), tt.wantError)
			}
		})
	}
}

func TestCELSemverRefVal(t *testing.T) {
	t.Parallel()

	sv, err := parseSemver("1.2.3")
	require.NoError(t, err)

	t.Run("ConvertToType StringType", func(t *testing.T) {
		t.Parallel()
		result := sv.ConvertToType(types.StringType)
		require.Equal(t, types.String("v1.2.3"), result)
	})

	t.Run("ConvertToType TypeType", func(t *testing.T) {
		t.Parallel()
		result := sv.ConvertToType(types.TypeType)
		require.Equal(t, celSemverRuntimeType, result)
	})

	t.Run("ConvertToType unsupported", func(t *testing.T) {
		t.Parallel()
		result := sv.ConvertToType(types.IntType)
		require.True(t, types.IsError(result))
	})

	t.Run("ConvertToNative returns error", func(t *testing.T) {
		t.Parallel()
		_, err := sv.ConvertToNative(nil)
		require.Error(t, err)
	})

	t.Run("Type returns semver type", func(t *testing.T) {
		t.Parallel()
		require.Equal(t, celSemverRuntimeType, sv.Type())
	})

	t.Run("Value returns canonical string", func(t *testing.T) {
		t.Parallel()
		require.Equal(t, "v1.2.3", sv.Value())
	})

	t.Run("Equal same version", func(t *testing.T) {
		t.Parallel()
		other, err := parseSemver("1.2.3")
		require.NoError(t, err)
		require.Equal(t, types.Bool(true), sv.Equal(other))
	})

	t.Run("Equal different version", func(t *testing.T) {
		t.Parallel()
		other, err := parseSemver("1.2.4")
		require.NoError(t, err)
		require.Equal(t, types.Bool(false), sv.Equal(other))
	})

	t.Run("Compare less", func(t *testing.T) {
		t.Parallel()
		other, err := parseSemver("2.0.0")
		require.NoError(t, err)
		require.Equal(t, types.Int(-1), sv.Compare(other))
	})

	t.Run("Compare greater", func(t *testing.T) {
		t.Parallel()
		other, err := parseSemver("1.0.0")
		require.NoError(t, err)
		require.Equal(t, types.Int(1), sv.Compare(other))
	})

	t.Run("Compare equal", func(t *testing.T) {
		t.Parallel()
		other, err := parseSemver("1.2.3")
		require.NoError(t, err)
		require.Equal(t, types.Int(0), sv.Compare(other))
	})

	t.Run("Equal wrong type", func(t *testing.T) {
		t.Parallel()
		result := sv.Equal(types.String("not-a-semver"))
		require.True(t, types.IsError(result))
	})

	t.Run("Compare wrong type", func(t *testing.T) {
		t.Parallel()
		result := sv.Compare(types.String("not-a-semver"))
		require.True(t, types.IsError(result))
	})
}
