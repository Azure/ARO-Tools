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

package imagemirror

import (
	"github.com/spf13/cobra"
)

// NewCommand returns the root imagemirror cobra.Command with subcommands for mirroring
// container images to an Azure Container Registry.
func NewCommand() (*cobra.Command, error) {
	cmd := &cobra.Command{
		Use:           "imagemirror",
		Short:         "Mirror container images to an Azure Container Registry.",
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	fromRegistry, err := newFromRegistryCommand()
	if err != nil {
		return nil, err
	}
	cmd.AddCommand(fromRegistry)

	fromOCILayout, err := newFromOCILayoutCommand()
	if err != nil {
		return nil, err
	}
	cmd.AddCommand(fromOCILayout)

	return cmd, nil
}
