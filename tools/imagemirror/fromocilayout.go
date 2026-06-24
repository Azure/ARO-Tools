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
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"

	"github.com/go-logr/logr"
	"github.com/spf13/cobra"
	"oras.land/oras-go/v2"
	"oras.land/oras-go/v2/content/oci"
	"oras.land/oras-go/v2/registry/remote"
	"oras.land/oras-go/v2/registry/remote/auth"

	"github.com/Azure/ARO-Tools/tools/cmdutils"
)

func newFromOCILayoutCommand() (*cobra.Command, error) {
	cmd := &cobra.Command{
		Use:           "from-oci-layout",
		Short:         "Mirror an image from a local OCI layout tar to an Azure Container Registry.",
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	opts := defaultFromOCILayoutOptions()
	if err := bindFromOCILayoutOptions(opts, cmd); err != nil {
		return nil, fmt.Errorf("failed to bind options: %w", err)
	}
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		ctx, cancel := signal.NotifyContext(cmd.Context(), os.Interrupt)
		defer cancel()

		validated, err := opts.Validate()
		if err != nil {
			return err
		}
		completed, err := validated.Complete(ctx)
		if err != nil {
			return err
		}
		return completed.Run(ctx)
	}

	return cmd, nil
}

// fromOCILayoutRawOptions holds raw input values for the from-oci-layout subcommand.
type fromOCILayoutRawOptions struct {
	*cmdutils.RawOptions

	TargetACR     string
	ACRSuffix     string
	Repository    string
	ImageTar      string
	ImageMetadata string
	DryRun        bool
}

func defaultFromOCILayoutOptions() *fromOCILayoutRawOptions {
	return &fromOCILayoutRawOptions{
		RawOptions: cmdutils.DefaultOptions(),
	}
}

func bindFromOCILayoutOptions(opts *fromOCILayoutRawOptions, cmd *cobra.Command) error {
	cmd.Flags().StringVar(&opts.TargetACR, "target-acr", opts.TargetACR, "Name of the target Azure Container Registry.")
	cmd.Flags().StringVar(&opts.ACRSuffix, "acr-suffix", opts.ACRSuffix, "DNS suffix for the ACR (e.g. .azurecr.io).")
	cmd.Flags().StringVar(&opts.Repository, "repository", opts.Repository, "Target image repository.")
	cmd.Flags().StringVar(&opts.ImageTar, "image-tar", opts.ImageTar, "Path to the OCI layout tar file.")
	cmd.Flags().StringVar(&opts.ImageMetadata, "image-metadata", opts.ImageMetadata, "Path to the image metadata JSON file.")
	cmd.Flags().BoolVar(&opts.DryRun, "dry-run", opts.DryRun, "Exit before copying, logging what would be done.")

	return cmdutils.BindOptions(opts.RawOptions, cmd)
}

// fromOCILayoutValidatedOptions is a private wrapper enforcing Validate() before Complete().
type fromOCILayoutValidatedOptions struct {
	*fromOCILayoutRawOptions
	*cmdutils.ValidatedOptions

	BuildTag string
}

// FromOCILayoutValidatedOptions wraps the validated options with a private pointer.
type FromOCILayoutValidatedOptions struct {
	*fromOCILayoutValidatedOptions
}

func (o *fromOCILayoutRawOptions) Validate() (*FromOCILayoutValidatedOptions, error) {
	for _, item := range []struct {
		flag  string
		name  string
		value *string
	}{
		{flag: "target-acr", name: "target ACR name", value: &o.TargetACR},
		{flag: "acr-suffix", name: "ACR DNS suffix", value: &o.ACRSuffix},
		{flag: "repository", name: "repository", value: &o.Repository},
		{flag: "image-tar", name: "OCI layout tar file", value: &o.ImageTar},
		{flag: "image-metadata", name: "image metadata file", value: &o.ImageMetadata},
	} {
		if item.value == nil || *item.value == "" {
			return nil, fmt.Errorf("the %s must be provided with --%s", item.name, item.flag)
		}
	}

	// Validate tar file exists.
	if _, err := os.Stat(o.ImageTar); err != nil {
		return nil, fmt.Errorf("image tar file does not exist at %s: %w", o.ImageTar, err)
	}

	// Validate metadata file exists and contains build_tag.
	if _, err := os.Stat(o.ImageMetadata); err != nil {
		return nil, fmt.Errorf("image metadata file does not exist at %s: %w", o.ImageMetadata, err)
	}

	metadataBytes, err := os.ReadFile(o.ImageMetadata)
	if err != nil {
		return nil, fmt.Errorf("failed to read image metadata file %s: %w", o.ImageMetadata, err)
	}

	var metadata struct {
		BuildTag string `json:"build_tag"`
	}
	if err := json.Unmarshal(metadataBytes, &metadata); err != nil {
		return nil, fmt.Errorf("failed to parse image metadata file %s: %w", o.ImageMetadata, err)
	}
	if metadata.BuildTag == "" {
		return nil, fmt.Errorf("build_tag not found in %s", o.ImageMetadata)
	}

	validated, err := o.RawOptions.Validate()
	if err != nil {
		return nil, err
	}

	return &FromOCILayoutValidatedOptions{
		fromOCILayoutValidatedOptions: &fromOCILayoutValidatedOptions{
			fromOCILayoutRawOptions: o,
			ValidatedOptions:        validated,
			BuildTag:                metadata.BuildTag,
		},
	}, nil
}

// fromOCILayoutCompletedOptions holds the fully resolved state for execution.
type fromOCILayoutCompletedOptions struct {
	TargetACRLoginServer string
	Repository           string
	ImageTar             string
	BuildTag             string
	DryRun               bool

	ACRCredential auth.Credential
}

// FromOCILayoutOptions wraps the completed options with a private pointer.
type FromOCILayoutOptions struct {
	*fromOCILayoutCompletedOptions
}

func (o *FromOCILayoutValidatedOptions) Complete(ctx context.Context) (*FromOCILayoutOptions, error) {
	completed, err := o.ValidatedOptions.Complete()
	if err != nil {
		return nil, err
	}

	targetLoginServer := o.TargetACR + o.ACRSuffix

	// Get Azure credentials and exchange for ACR refresh token.
	cred, err := cmdutils.GetAzureTokenCredentialsForCloud(completed.Configuration)
	if err != nil {
		return nil, fmt.Errorf("failed to create Azure credentials: %w", err)
	}

	acrToken, err := exchangeACRAccessTokenWithRetry(ctx, cred, targetLoginServer)
	if err != nil {
		return nil, fmt.Errorf("failed to authenticate to target ACR %s: %w", targetLoginServer, err)
	}

	acrCredential := auth.Credential{
		Username: "00000000-0000-0000-0000-000000000000",
		Password: acrToken.Token,
	}

	return &FromOCILayoutOptions{
		fromOCILayoutCompletedOptions: &fromOCILayoutCompletedOptions{
			TargetACRLoginServer: targetLoginServer,
			Repository:           o.Repository,
			ImageTar:             o.ImageTar,
			BuildTag:             o.BuildTag,
			DryRun:               o.DryRun,
			ACRCredential:        acrCredential,
		},
	}, nil
}

func (o *FromOCILayoutOptions) Run(ctx context.Context) error {
	logger, err := logr.FromContext(ctx)
	if err != nil {
		return fmt.Errorf("failed to get logger from context: %w", err)
	}

	targetImage := fmt.Sprintf("%s/%s:%s", o.TargetACRLoginServer, o.Repository, o.BuildTag)

	if o.DryRun {
		logger.Info("DRY_RUN is enabled. Exiting without making changes.", "source", o.ImageTar, "tag", o.BuildTag, "target", targetImage)
		return nil
	}

	logger.Info("Mirroring image from OCI layout.", "source", o.ImageTar, "tag", o.BuildTag, "target", targetImage)

	// Load the OCI layout from tar.
	srcStore, err := oci.NewFromTar(ctx, o.ImageTar)
	if err != nil {
		return fmt.Errorf("failed to load OCI layout from %s: %w", o.ImageTar, err)
	}

	// Configure target repository.
	dstRepo, err := remote.NewRepository(fmt.Sprintf("%s/%s", o.TargetACRLoginServer, o.Repository))
	if err != nil {
		return fmt.Errorf("failed to create target repository client: %w", err)
	}
	dstRepo.Client = &auth.Client{
		Credential: auth.StaticCredential(o.TargetACRLoginServer, o.ACRCredential),
	}

	// Copy from OCI layout to remote registry.
	desc, err := oras.Copy(ctx, srcStore, o.BuildTag, dstRepo, o.BuildTag, oras.DefaultCopyOptions)
	if err != nil {
		return fmt.Errorf("failed to copy image from OCI layout to %s: %w", targetImage, err)
	}

	logger.Info("Successfully mirrored image from OCI layout.", "target", targetImage, "digest", desc.Digest.String(), "mediaType", desc.MediaType, "size", desc.Size)
	return nil
}
