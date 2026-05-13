package sync

import (
	"fmt"

	"github.com/go-logr/logr"
	"github.com/spf13/cobra"
)

// RawOptions holds the raw CLI input values.
type RawOptions struct {
	TargetACR             string
	SourceRegistry        string
	Repository            string
	Digest                string
	CopyFrom              string
	ImageFilePath         string
	ImageTarFileName      string
	ImageMetadataFileName string
	ImageTarSAS           string
	ImageMetadataSAS      string
	PullSecretKV          string
	PullSecretName        string
	DryRun                bool
}

// validatedOptions is a private wrapper that enforces a call of Validate() before Complete() can be invoked.
type validatedOptions struct {
	*RawOptions
}

// ValidatedOptions wraps validatedOptions to enforce the Validate() -> Complete() flow.
type ValidatedOptions struct {
	*validatedOptions
}

// completedOptions holds the finalized, ready-to-use options.
type completedOptions struct {
	TargetACR             string
	SourceRegistry        string
	Repository            string
	Digest                string
	CopyFrom              string
	ImageFilePath         string
	ImageTarFileName      string
	ImageMetadataFileName string
	ImageTarSAS           string
	ImageMetadataSAS      string
	PullSecretKV          string
	PullSecretName        string
	DryRun                bool
}

// Options wraps completedOptions to enforce the Validate() -> Complete() -> Run() flow.
type Options struct {
	*completedOptions
}

// DefaultOptions returns a new RawOptions with defaults.
func DefaultOptions() *RawOptions {
	return &RawOptions{}
}

// BindOptions binds CLI flags to the raw options.
func BindOptions(opts *RawOptions, cmd *cobra.Command) {
	cmd.Flags().StringVar(&opts.TargetACR, "target-acr", opts.TargetACR, "Target Azure Container Registry name.")
	cmd.Flags().StringVar(&opts.SourceRegistry, "source-registry", opts.SourceRegistry, "Source container registry host (for registry copy mode).")
	cmd.Flags().StringVar(&opts.Repository, "repository", opts.Repository, "Image repository name.")
	cmd.Flags().StringVar(&opts.Digest, "digest", opts.Digest, "Image digest (e.g. sha256:...).")
	cmd.Flags().StringVar(&opts.CopyFrom, "copy-from", opts.CopyFrom, "Copy mode: 'oci-layout' for file-based or empty for registry-based.")
	cmd.Flags().StringVar(&opts.ImageFilePath, "image-file-path", opts.ImageFilePath, "Directory path containing image tar and metadata files.")
	cmd.Flags().StringVar(&opts.ImageTarFileName, "image-tar-file", opts.ImageTarFileName, "Image tar file name for OCI layout mode.")
	cmd.Flags().StringVar(&opts.ImageMetadataFileName, "image-metadata-file", opts.ImageMetadataFileName, "Image metadata JSON file name for OCI layout mode.")
	cmd.Flags().StringVar(&opts.ImageTarSAS, "image-tar-sas", opts.ImageTarSAS, "SAS URL for downloading the image tar file.")
	cmd.Flags().StringVar(&opts.ImageMetadataSAS, "image-metadata-sas", opts.ImageMetadataSAS, "SAS URL for downloading the image metadata file.")
	cmd.Flags().StringVar(&opts.PullSecretKV, "pull-secret-kv", opts.PullSecretKV, "KeyVault name containing pull secret (for registry copy mode).")
	cmd.Flags().StringVar(&opts.PullSecretName, "pull-secret", opts.PullSecretName, "Pull secret name in KeyVault (for registry copy mode).")
	cmd.Flags().BoolVar(&opts.DryRun, "dry-run", opts.DryRun, "If true, validate inputs without making changes.")
}

// Validate validates the raw options.
func (o *RawOptions) Validate() (*ValidatedOptions, error) {
	if o.TargetACR == "" {
		return nil, fmt.Errorf("the target ACR must be provided with --target-acr")
	}
	if o.Repository == "" {
		return nil, fmt.Errorf("the repository must be provided with --repository")
	}

	if o.CopyFrom == copyFromOCI {
		if o.ImageTarFileName == "" {
			return nil, fmt.Errorf("the image tar file name must be provided with --image-tar-file for oci-layout mode")
		}
		if o.ImageMetadataFileName == "" {
			return nil, fmt.Errorf("the image metadata file name must be provided with --image-metadata-file for oci-layout mode")
		}
	} else {
		if o.SourceRegistry == "" {
			return nil, fmt.Errorf("the source registry must be provided with --source-registry for registry mode")
		}
		if o.Digest == "" {
			return nil, fmt.Errorf("the digest must be provided with --digest for registry mode")
		}
		if o.PullSecretKV == "" {
			return nil, fmt.Errorf("the pull secret KeyVault must be provided with --pull-secret-kv for registry mode")
		}
		if o.PullSecretName == "" {
			return nil, fmt.Errorf("the pull secret name must be provided with --pull-secret for registry mode")
		}
	}

	return &ValidatedOptions{
		validatedOptions: &validatedOptions{
			RawOptions: o,
		},
	}, nil
}

// Complete builds the finalized options.
func (o *ValidatedOptions) Complete() (*Options, error) {
	return &Options{
		completedOptions: &completedOptions{
			TargetACR:             o.TargetACR,
			SourceRegistry:        o.SourceRegistry,
			Repository:            o.Repository,
			Digest:                o.Digest,
			CopyFrom:              o.CopyFrom,
			ImageFilePath:         o.ImageFilePath,
			ImageTarFileName:      o.ImageTarFileName,
			ImageMetadataFileName: o.ImageMetadataFileName,
			ImageTarSAS:           o.ImageTarSAS,
			ImageMetadataSAS:      o.ImageMetadataSAS,
			PullSecretKV:          o.PullSecretKV,
			PullSecretName:        o.PullSecretName,
			DryRun:                o.DryRun,
		},
	}, nil
}

// NewRunner creates a runner from completed options.
func (o *Options) NewRunner(logger logr.Logger) *Runner {
	return &Runner{
		opts:   o,
		logger: logger,
	}
}
