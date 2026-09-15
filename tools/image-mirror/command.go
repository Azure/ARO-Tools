package imagemirror

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/Azure/ARO-Tools/tools/image-mirror/sync"
)

func NewCommand() (*cobra.Command, error) {
	cmd := &cobra.Command{
		Use:           "image-mirror",
		Short:         "Mirror container images to Azure Container Registry.",
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	commands := []func() (*cobra.Command, error){
		sync.NewCommand,
	}
	for _, newCmd := range commands {
		c, err := newCmd()
		if err != nil {
			return nil, fmt.Errorf("failed to create subcommand: %w", err)
		}
		cmd.AddCommand(c)
	}

	return cmd, nil
}
