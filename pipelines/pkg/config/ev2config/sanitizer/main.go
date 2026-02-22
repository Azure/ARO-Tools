package main

import (
	"log/slog"
	"os"

	"github.com/spf13/cobra"
)

func main() {
	cmd := &cobra.Command{
		Use:          "sanitizer",
		Short:        "Sanitize Ev2 central configuration into a service-configuration compatible file.",
		SilenceUsage: true,
	}

	opts := DefaultOptions()
	if err := BindOptions(opts, cmd); err != nil {
		slog.Error("Failed to bind options.", "err", err)
		os.Exit(1)
	}
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		validated, err := opts.Validate()
		if err != nil {
			return err
		}
		completed, err := validated.Complete()
		if err != nil {
			return err
		}
		return completed.Sanitize()
	}

	if err := cmd.Execute(); err != nil {
		slog.Error("Command failed.", "err", err)
		os.Exit(1)
	}
}
