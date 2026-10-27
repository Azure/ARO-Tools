// Copyright 2026 Microsoft Corporation
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

package istio

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDiscoveryCommandRegistration(t *testing.T) {
	cmd, err := NewCommand()
	require.NoError(t, err)
	discovery, args, err := cmd.Find([]string{"discover-bootstrap"})
	require.NoError(t, err)
	require.Empty(t, args)
	require.Equal(t, "discover-bootstrap", discovery.Name())
	require.Nil(t, discovery.Flags().Lookup("kubeconfig"), "discovery must not require Kubernetes access")
	require.NotNil(t, discovery.Flags().Lookup("versions"))
	require.NotNil(t, discovery.Flags().Lookup("versions-json"))
	require.NotNil(t, discovery.Flags().Lookup("output-file"))
}
