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

package yamlwrap

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Azure/ARO-Tools/internal/testutil"
)

func TestWrapYAML(t *testing.T) {
	inputPath := filepath.Join("testdata", "unwrapped.yaml")
	data, err := os.ReadFile(inputPath)
	require.NoError(t, err)

	result, err := WrapYAML(data)
	require.NoError(t, err)

	testutil.CompareWithFixture(t, result, testutil.WithExtension(".yaml"))
}

func TestUnwrapYAML(t *testing.T) {
	inputPath := filepath.Join("testdata", "wrapped.yaml")
	data, err := os.ReadFile(inputPath)
	require.NoError(t, err)

	result, err := UnwrapYAML(data)
	require.NoError(t, err)

	testutil.CompareWithFixture(t, result, testutil.WithExtension(".yaml"))
}

func TestRoundTrip(t *testing.T) {
	// Read the unwrapped.yaml file
	inputPath := filepath.Join("testdata", "unwrapped.yaml")
	original, err := os.ReadFile(inputPath)
	require.NoError(t, err)

	// wrap it in memory
	wrapped, err := WrapYAML(original)
	require.NoError(t, err)

	// ... then unwrap
	result, err := UnwrapYAML(wrapped)
	require.NoError(t, err)

	// Compare that the round trip produces the same result as the original
	assert.Empty(t, cmp.Diff(original, result))
}
