package main

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"sigs.k8s.io/yaml"
)

func DefaultOptions() *RawOptions {
	return &RawOptions{}
}

func BindOptions(opts *RawOptions, cmd *cobra.Command) error {
	cmd.Flags().StringArrayVar(&opts.Ev2Configurations, "input", opts.Ev2Configurations, "Path to an input Ev2 central configuration file.")
	cmd.Flags().StringVar(&opts.OutputFile, "output", opts.OutputFile, "File to write output configuration to.")

	for _, flag := range []string{"input", "output"} {
		if err := cmd.MarkFlagFilename(flag); err != nil {
			return fmt.Errorf("failed to mark flag %q as a file: %w", flag, err)
		}
	}
	return nil
}

// RawOptions holds input values.
type RawOptions struct {
	Ev2Configurations []string
	OutputFile        string
}

// validatedOptions is a private wrapper that enforces a call of Validate() before Complete() can be invoked.
type validatedOptions struct {
	*RawOptions
}

type ValidatedOptions struct {
	// Embed a private pointer that cannot be instantiated outside of this package.
	*validatedOptions
}

// completedOptions is a private wrapper that enforces a call of Complete() before config generation can be invoked.
type completedOptions struct {
	ConfigByCloud map[string]CentralConfig
	Output        io.WriteCloser
}

type Options struct {
	// Embed a private pointer that cannot be instantiated outside of this package.
	*completedOptions
}

func (o *RawOptions) Validate() (*ValidatedOptions, error) {
	if len(o.Ev2Configurations) == 0 {
		return nil, errors.New("central Ev2 configuration(s) must be provided with --input")
	}

	if len(o.OutputFile) == 0 {
		return nil, errors.New("output file must be provided with --output")
	}

	return &ValidatedOptions{
		validatedOptions: &validatedOptions{
			RawOptions: o,
		},
	}, nil
}

func (o *ValidatedOptions) Complete() (*Options, error) {
	configByCloud := map[string]CentralConfig{}
	for _, config := range o.Ev2Configurations {
		if !strings.HasSuffix(config, ".config.json") {
			return nil, fmt.Errorf("config file %s does not match <cloud>.config.json pattern", config)
		}
		cloud := strings.TrimSuffix(config, ".config.json")

		raw, err := os.ReadFile(config)
		if err != nil {
			return nil, fmt.Errorf("failed to read config file %s: %w", config, err)
		}

		var cfg CentralConfig
		if err := yaml.Unmarshal(raw, &cfg); err != nil {
			return nil, fmt.Errorf("failed to unmarshal config file %s: %w", config, err)
		}

		configByCloud[cloud] = cfg
	}

	output, err := os.Create(o.OutputFile)
	if err != nil {
		return nil, fmt.Errorf("failed to open output file %s: %w", o.OutputFile, err)
	}

	return &Options{
		completedOptions: &completedOptions{
			ConfigByCloud: configByCloud,
			Output:        output,
		},
	}, nil
}

func (opts *Options) Sanitize() error {
	output := Sanitize(opts.ConfigByCloud)

	defer func() {
		if err := opts.Output.Close(); err != nil {
			slog.Error("failed to close output file", "error", err)
		}
	}()

	encoded, err := yaml.Marshal(output)
	if err != nil {
		return fmt.Errorf("failed to marshal output: %w", err)
	}

	if _, err := opts.Output.Write(encoded); err != nil {
		return fmt.Errorf("failed to write output: %w", err)
	}
	return nil
}
