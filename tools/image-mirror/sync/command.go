package sync

import (
	"fmt"
	"log"
	"os"
	"os/signal"

	"github.com/go-logr/logr"
	"github.com/go-logr/logr/funcr"
	"github.com/spf13/cobra"
)

// NewCommand creates the "sync" subcommand.
func NewCommand() (*cobra.Command, error) {
	cmd := &cobra.Command{
		Use:           "sync",
		Short:         "Sync a container image to an Azure Container Registry.",
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	opts := DefaultOptions()
	BindOptions(opts, cmd)
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		ctx, cancel := signal.NotifyContext(cmd.Context(), os.Interrupt)
		defer cancel()

		validated, err := opts.Validate()
		if err != nil {
			return fmt.Errorf("validation failed: %w", err)
		}
		completed, err := validated.Complete()
		if err != nil {
			return fmt.Errorf("completion failed: %w", err)
		}

		logger := newLogger()
		runner := completed.NewRunner(logger)
		return runner.Run(ctx)
	}

	return cmd, nil
}

func newLogger() logr.Logger {
	return funcr.New(func(prefix, args string) {
		if prefix != "" {
			log.Printf("%s: %s", prefix, args)
		} else {
			log.Print(args)
		}
	}, funcr.Options{
		Verbosity: 1,
	})
}
