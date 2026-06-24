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
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"strings"

	"github.com/go-logr/logr"
	"github.com/spf13/cobra"
	"oras.land/oras-go/v2"
	"oras.land/oras-go/v2/registry/remote"
	"oras.land/oras-go/v2/registry/remote/auth"
	"oras.land/oras-go/v2/registry/remote/credentials"

	"github.com/Azure/ARO-Tools/tools/cmdutils"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/cloud"
	"github.com/Azure/azure-sdk-for-go/sdk/security/keyvault/azsecrets"
)

func newFromRegistryCommand() (*cobra.Command, error) {
	cmd := &cobra.Command{
		Use:           "from-registry",
		Short:         "Mirror an image from a source registry to an Azure Container Registry.",
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	opts := defaultFromRegistryOptions()
	if err := bindFromRegistryOptions(opts, cmd); err != nil {
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

// fromRegistryRawOptions holds raw input values for the from-registry subcommand.
type fromRegistryRawOptions struct {
	*cmdutils.RawOptions

	TargetACR      string
	ACRSuffix      string
	SourceRegistry string
	Repository     string
	Digest         string
	DryRun         bool

	// Auth flags - mutually exclusive
	AuthAnonymous      bool
	AuthSourceConfig   string
	AuthPullSecretKV   string
	AuthPullSecretName string
}

func defaultFromRegistryOptions() *fromRegistryRawOptions {
	return &fromRegistryRawOptions{
		RawOptions: cmdutils.DefaultOptions(),
	}
}

func bindFromRegistryOptions(opts *fromRegistryRawOptions, cmd *cobra.Command) error {
	cmd.Flags().StringVar(&opts.TargetACR, "target-acr", opts.TargetACR, "Name of the target Azure Container Registry.")
	cmd.Flags().StringVar(&opts.ACRSuffix, "acr-suffix", opts.ACRSuffix, "DNS suffix for the ACR (e.g. .azurecr.io).")
	cmd.Flags().StringVar(&opts.SourceRegistry, "source-registry", opts.SourceRegistry, "Hostname of the source registry.")
	cmd.Flags().StringVar(&opts.Repository, "repository", opts.Repository, "Image repository to mirror.")
	cmd.Flags().StringVar(&opts.Digest, "digest", opts.Digest, "Image digest (e.g. sha256:abc123...).")
	cmd.Flags().BoolVar(&opts.DryRun, "dry-run", opts.DryRun, "Exit before copying, logging what would be done.")

	cmd.Flags().BoolVar(&opts.AuthAnonymous, "auth.anonymous", opts.AuthAnonymous, "Pull from the source registry without authentication.")
	cmd.Flags().StringVar(&opts.AuthSourceConfig, "auth.source-config", opts.AuthSourceConfig, "Path to a Docker auth config JSON for the source registry.")
	cmd.Flags().StringVar(&opts.AuthPullSecretKV, "auth.pull-secret.keyvault", opts.AuthPullSecretKV, "Key Vault name containing the pull secret.")
	cmd.Flags().StringVar(&opts.AuthPullSecretName, "auth.pull-secret.name", opts.AuthPullSecretName, "Name of the pull secret in Key Vault.")

	return cmdutils.BindOptions(opts.RawOptions, cmd)
}

// fromRegistryValidatedOptions is a private wrapper enforcing Validate() before Complete().
type fromRegistryValidatedOptions struct {
	*fromRegistryRawOptions
	*cmdutils.ValidatedOptions
}

// FromRegistryValidatedOptions wraps the validated options with a private pointer.
type FromRegistryValidatedOptions struct {
	*fromRegistryValidatedOptions
}

func (o *fromRegistryRawOptions) Validate() (*FromRegistryValidatedOptions, error) {
	for _, item := range []struct {
		flag  string
		name  string
		value *string
	}{
		{flag: "target-acr", name: "target ACR name", value: &o.TargetACR},
		{flag: "acr-suffix", name: "ACR DNS suffix", value: &o.ACRSuffix},
		{flag: "source-registry", name: "source registry", value: &o.SourceRegistry},
		{flag: "repository", name: "repository", value: &o.Repository},
		{flag: "digest", name: "digest", value: &o.Digest},
	} {
		if item.value == nil || *item.value == "" {
			return nil, fmt.Errorf("the %s must be provided with --%s", item.name, item.flag)
		}
	}

	// Validate mutually exclusive auth flags.
	authModes := 0
	if o.AuthAnonymous {
		authModes++
	}
	if o.AuthSourceConfig != "" {
		authModes++
	}
	if o.AuthPullSecretKV != "" || o.AuthPullSecretName != "" {
		authModes++
	}
	if authModes == 0 {
		return nil, fmt.Errorf("exactly one auth mode must be specified: --auth.anonymous, --auth.source-config, or --auth.pull-secret.{keyvault,name}")
	}
	if authModes > 1 {
		return nil, fmt.Errorf("auth modes are mutually exclusive: --auth.anonymous, --auth.source-config, and --auth.pull-secret.{keyvault,name} cannot be combined")
	}

	// If pull secret mode, both flags are required.
	if (o.AuthPullSecretKV != "") != (o.AuthPullSecretName != "") {
		return nil, fmt.Errorf("both --auth.pull-secret.keyvault and --auth.pull-secret.name must be provided together")
	}

	// Validate digest format: must be algorithm:hex.
	if !strings.Contains(o.Digest, ":") {
		return nil, fmt.Errorf("invalid --digest %q: expected format algorithm:hex (e.g. sha256:abc123...)", o.Digest)
	}
	algorithm, hex, _ := strings.Cut(o.Digest, ":")
	if algorithm != "sha256" {
		return nil, fmt.Errorf("unsupported digest algorithm %q in --digest: only sha256 is supported", algorithm)
	}
	if hex == "" {
		return nil, fmt.Errorf("invalid --digest %q: hex portion after algorithm prefix must not be empty", o.Digest)
	}

	validated, err := o.RawOptions.Validate()
	if err != nil {
		return nil, err
	}

	return &FromRegistryValidatedOptions{
		fromRegistryValidatedOptions: &fromRegistryValidatedOptions{
			fromRegistryRawOptions: o,
			ValidatedOptions:       validated,
		},
	}, nil
}

// fromRegistryCompletedOptions holds the fully resolved state for execution.
type fromRegistryCompletedOptions struct {
	TargetACRLoginServer string
	SourceRegistry       string
	Repository           string
	Digest               string
	DryRun               bool

	ACRCredential  auth.Credential
	SourceCredFunc auth.CredentialFunc
}

// FromRegistryOptions wraps the completed options with a private pointer.
type FromRegistryOptions struct {
	*fromRegistryCompletedOptions
}

func (o *FromRegistryValidatedOptions) Complete(ctx context.Context) (*FromRegistryOptions, error) {
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

	// Resolve source registry credentials.
	var sourceCredFunc auth.CredentialFunc
	switch {
	case o.AuthAnonymous:
		sourceCredFunc = auth.StaticCredential(o.SourceRegistry, auth.EmptyCredential)
	case o.AuthSourceConfig != "":
		store, err := credentials.NewStore(o.AuthSourceConfig, credentials.StoreOptions{})
		if err != nil {
			return nil, fmt.Errorf("failed to load source auth config from %s: %w", o.AuthSourceConfig, err)
		}
		sourceCredFunc = credentials.Credential(store)
	case o.AuthPullSecretKV != "":
		dockerConfig, err := fetchPullSecretFromKeyVault(ctx, cred, completed.Configuration, o.AuthPullSecretKV, o.AuthPullSecretName)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch pull secret from Key Vault: %w", err)
		}
		store := credentials.NewMemoryStore()
		if err := loadDockerConfigIntoStore(ctx, store, dockerConfig); err != nil {
			return nil, fmt.Errorf("failed to parse pull secret as Docker auth config: %w", err)
		}
		sourceCredFunc = credentials.Credential(store)
	}

	return &FromRegistryOptions{
		fromRegistryCompletedOptions: &fromRegistryCompletedOptions{
			TargetACRLoginServer: targetLoginServer,
			SourceRegistry:       o.SourceRegistry,
			Repository:           o.Repository,
			Digest:               o.Digest,
			DryRun:               o.DryRun,
			ACRCredential:        acrCredential,
			SourceCredFunc:       sourceCredFunc,
		},
	}, nil
}

func (o *FromRegistryOptions) Run(ctx context.Context) error {
	logger, err := logr.FromContext(ctx)
	if err != nil {
		return fmt.Errorf("failed to get logger from context: %w", err)
	}

	// Short-circuit if source and target are the same.
	if o.SourceRegistry == o.TargetACRLoginServer {
		logger.Info("Source and target registry are the same. No mirroring needed.")
		return nil
	}

	srcImage := fmt.Sprintf("%s/%s@%s", o.SourceRegistry, o.Repository, o.Digest)
	digestNoPrefix := strings.TrimPrefix(o.Digest, "sha256:")
	targetImage := fmt.Sprintf("%s/%s:%s", o.TargetACRLoginServer, o.Repository, digestNoPrefix)

	if o.DryRun {
		logger.Info("DRY_RUN is enabled. Exiting without making changes.", "source", srcImage, "target", targetImage)
		return nil
	}

	logger.Info("Mirroring image.", "source", srcImage, "target", targetImage)
	logger.Info("The image will still be available under its original digest in the target registry.", "digest", o.Digest)

	// Configure source repository.
	srcRepo, err := remote.NewRepository(fmt.Sprintf("%s/%s", o.SourceRegistry, o.Repository))
	if err != nil {
		return fmt.Errorf("failed to create source repository client: %w", err)
	}
	srcRepo.Client = &auth.Client{
		Credential: o.SourceCredFunc,
	}

	// Configure target repository.
	dstRepo, err := remote.NewRepository(fmt.Sprintf("%s/%s", o.TargetACRLoginServer, o.Repository))
	if err != nil {
		return fmt.Errorf("failed to create target repository client: %w", err)
	}
	dstRepo.Client = &auth.Client{
		Credential: auth.StaticCredential(o.TargetACRLoginServer, o.ACRCredential),
	}

	// Copy the image.
	desc, err := oras.Copy(ctx, srcRepo, o.Digest, dstRepo, digestNoPrefix, oras.DefaultCopyOptions)
	if err != nil {
		return fmt.Errorf("failed to copy image from %s to %s: %w", srcImage, targetImage, err)
	}

	logger.Info("Successfully mirrored image.", "target", targetImage, "digest", desc.Digest.String(), "mediaType", desc.MediaType, "size", desc.Size)
	return nil
}

// fetchPullSecretFromKeyVault fetches a pull secret from Azure Key Vault and base64-decodes it.
func fetchPullSecretFromKeyVault(ctx context.Context, cred azcore.TokenCredential, cloudConfig cloud.Configuration, vaultName, secretName string) ([]byte, error) {
	vaultURI := fmt.Sprintf("https://%s.vault.azure.net", vaultName)
	client, err := azsecrets.NewClient(vaultURI, cred, &azsecrets.ClientOptions{
		ClientOptions: azcore.ClientOptions{Cloud: cloudConfig},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create Key Vault secrets client: %w", err)
	}

	resp, err := client.GetSecret(ctx, secretName, "", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get secret %s from vault %s: %w", secretName, vaultName, err)
	}

	if resp.Value == nil {
		return nil, fmt.Errorf("secret %s in vault %s has no value", secretName, vaultName)
	}

	decoded, err := base64.StdEncoding.DecodeString(*resp.Value)
	if err != nil {
		return nil, fmt.Errorf("failed to base64-decode pull secret: %w", err)
	}

	return decoded, nil
}

// dockerConfig represents the structure of a Docker auth config JSON.
type dockerConfig struct {
	Auths map[string]dockerAuthEntry `json:"auths"`
}

type dockerAuthEntry struct {
	Auth string `json:"auth"`
}

// loadDockerConfigIntoStore parses a Docker auth config JSON and loads credentials into a memory store.
func loadDockerConfigIntoStore(ctx context.Context, store credentials.Store, configData []byte) error {
	var config dockerConfig
	if err := json.Unmarshal(configData, &config); err != nil {
		return fmt.Errorf("failed to unmarshal Docker auth config: %w", err)
	}

	for registry, entry := range config.Auths {
		decoded, err := base64.StdEncoding.DecodeString(entry.Auth)
		if err != nil {
			return fmt.Errorf("failed to decode auth for registry %s: %w", registry, err)
		}

		parts := strings.SplitN(string(decoded), ":", 2)
		if len(parts) != 2 {
			return fmt.Errorf("invalid auth format for registry %s", registry)
		}

		cred := auth.Credential{
			Username: parts[0],
			Password: parts[1],
		}
		if err := store.Put(ctx, registry, cred); err != nil {
			return fmt.Errorf("failed to store credential for registry %s: %w", registry, err)
		}
	}

	return nil
}
