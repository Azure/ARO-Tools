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
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

func DefaultOptions() *RawOptions {
	return &RawOptions{
		ReplaceOutput: false,
	}
}

func BindOptions(opts *RawOptions, cmd *cobra.Command) error {
	cmd.Flags().StringVar(&opts.InputPath, "input", opts.InputPath, "Path to the input file.")
	cmd.Flags().StringVar(&opts.OutputPath, "output", opts.OutputPath, "Path to the output file (defaults to input file).")
	cmd.Flags().BoolVar(&opts.ReplaceOutput, "replace-output", opts.ReplaceOutput, "Replace the output file if it exists.")
	for _, flag := range []string{
		"input",
		"output",
	} {
		if err := cmd.MarkFlagFilename(flag); err != nil {
			return fmt.Errorf("failed to mark flag %q as a file: %w", flag, err)
		}
	}
	return nil
}

// RawOptions holds input values.
type RawOptions struct {
	InputPath     string
	OutputPath    string
	ReplaceOutput bool
}

// validatedOptions is a private wrapper that enforces a call of Validate() before Complete() can be invoked.
type validatedOptions struct {
	InputPath  string
	OutputPath string
}

type ValidatedOptions struct {
	// Embed a private pointer that cannot be instantiated outside of this package.
	*validatedOptions
}

// completedOptions is a private wrapper that enforces a call of Complete() before Config generation can be invoked.
type completedOptions struct {
	InputPath  string
	OutputPath string
}

type Options struct {
	// Embed a private pointer that cannot be instantiated outside of this package.
	*completedOptions
}

func (o *RawOptions) Validate() (*ValidatedOptions, error) {
	// input file must be specified
	if o.InputPath == "" {
		return nil, fmt.Errorf("the file to process must be provided with --input")
	}

	// ... and must exist
	if _, err := os.Stat(o.InputPath); err != nil {
		return nil, fmt.Errorf("input file %q does not exist: %w", o.InputPath, err)
	}

	// default output to input if not specified
	outputPath := o.OutputPath
	if outputPath == "" {
		outputPath = o.InputPath
	}

	// check if output file exists and handle replacement
	if _, err := os.Stat(outputPath); err == nil && !o.ReplaceOutput {
		return nil, fmt.Errorf("output file %q already exists, use --replace-output to overwrite", outputPath)
	}

	return &ValidatedOptions{
		validatedOptions: &validatedOptions{
			InputPath:  o.InputPath,
			OutputPath: outputPath,
		},
	}, nil
}

func (o *ValidatedOptions) Complete() (*Options, error) {
	return &Options{
		completedOptions: &completedOptions{
			InputPath:  o.InputPath,
			OutputPath: o.OutputPath,
		},
	}, nil
}

func (opts *Options) Wrap(ctx context.Context) error {
	return WrapFile(opts.InputPath, opts.OutputPath)
}

func (opts *Options) Unwrap(ctx context.Context) error {
	return UnwrapFile(opts.InputPath, opts.OutputPath)
}
