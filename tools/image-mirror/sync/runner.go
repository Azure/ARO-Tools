package sync

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-logr/logr"
)

const (
	maxRetries   = 5
	orasUsername = "00000000-0000-0000-0000-000000000000"
	copyFromOCI  = "oci-layout"
)

// Runner executes the image mirror sync operation.
type Runner struct {
	opts   *Options
	logger logr.Logger
}

// Run executes the image mirror sync operation.
func (r *Runner) Run(ctx context.Context) error {
	if r.opts.CopyFrom == copyFromOCI {
		return r.copyImageFromOCILayout(ctx)
	}
	return r.copyImageFromRegistry(ctx)
}

func (r *Runner) copyImageFromRegistry(ctx context.Context) error {
	// Check if source and target are the same
	acrDomainSuffix, err := r.getACRDomainSuffix(ctx)
	if err != nil {
		return fmt.Errorf("failed to get ACR domain suffix: %w", err)
	}
	if r.opts.SourceRegistry == r.opts.TargetACR+acrDomainSuffix {
		r.logger.Info("Source and target registry are the same. No mirroring needed.")
		return nil
	}

	// Create temp directory for auth config
	tmpDir, err := os.MkdirTemp("", "image-mirror-*")
	if err != nil {
		return fmt.Errorf("failed to create temp directory: %w", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	containersDir := filepath.Join(tmpDir, "containers")
	if err := os.MkdirAll(containersDir, 0o700); err != nil {
		return fmt.Errorf("failed to create containers directory: %w", err)
	}
	authJSON := filepath.Join(containersDir, "auth.json")

	// Fetch pull secret from KeyVault
	r.logger.Info("Fetching pull secret from KeyVault", "vault", r.opts.PullSecretKV, "secret", r.opts.PullSecretName)
	if err := r.runCommand(ctx, "az", "keyvault", "secret", "download",
		"--vault-name", r.opts.PullSecretKV,
		"--name", r.opts.PullSecretName,
		"-e", "base64",
		"--file", authJSON,
	); err != nil {
		return fmt.Errorf("failed to download pull secret: %w", err)
	}

	// ACR login to target registry
	r.logger.Info("Logging into target ACR", "acr", r.opts.TargetACR)
	loginServer, accessToken, err := r.acrLogin(ctx)
	if err != nil {
		return fmt.Errorf("failed to login to ACR: %w", err)
	}

	// oras login to target
	if err := r.orasLogin(ctx, loginServer, accessToken, authJSON); err != nil {
		return fmt.Errorf("failed to oras login to target ACR: %w", err)
	}

	if r.opts.DryRun {
		r.logger.Info("DRY_RUN is enabled. Exiting without making changes.")
		return nil
	}

	// Mirror image
	digestNoPrefix := strings.TrimPrefix(r.opts.Digest, "sha256:")
	srcImage := fmt.Sprintf("%s/%s@%s", r.opts.SourceRegistry, r.opts.Repository, r.opts.Digest)
	targetImage := fmt.Sprintf("%s/%s:%s", loginServer, r.opts.Repository, digestNoPrefix)
	r.logger.Info("Mirroring image", "src", srcImage, "target", targetImage)
	r.logger.Info("The image will still be available under its original digest in the target registry", "digest", r.opts.Digest)

	return r.runCommand(ctx, "oras", "cp", srcImage, targetImage,
		"--from-registry-config", authJSON,
		"--to-registry-config", authJSON,
	)
}

func (r *Runner) copyImageFromOCILayout(ctx context.Context) error {
	imageTarFile, imageMetadataFile, err := r.resolveImageFiles(ctx)
	if err != nil {
		return err
	}

	// Read build_tag from metadata
	buildTag, err := r.readBuildTag(imageMetadataFile)
	if err != nil {
		return err
	}
	r.logger.Info("Resolved build tag", "buildTag", buildTag)

	// Get ACR login server
	acrDomainSuffix, err := r.getACRDomainSuffix(ctx)
	if err != nil {
		return fmt.Errorf("failed to get ACR domain suffix: %w", err)
	}
	targetACRLoginServer := r.opts.TargetACR + acrDomainSuffix

	// ACR login
	r.logger.Info("Logging into target ACR", "acr", r.opts.TargetACR)
	_, accessToken, err := r.acrLogin(ctx)
	if err != nil {
		return fmt.Errorf("failed to login to ACR: %w", err)
	}

	// oras login
	if err := r.orasLogin(ctx, targetACRLoginServer, accessToken, ""); err != nil {
		return fmt.Errorf("failed to oras login to target ACR: %w", err)
	}

	if r.opts.DryRun {
		r.logger.Info("DRY_RUN is enabled. Exiting without making changes.")
		return nil
	}

	// Copy from OCI layout
	targetImage := fmt.Sprintf("%s/%s:%s", targetACRLoginServer, r.opts.Repository, buildTag)
	r.logger.Info("Copying image from OCI layout", "source", imageTarFile, "target", targetImage)
	return r.runCommand(ctx, "oras", "cp",
		"--from-oci-layout", fmt.Sprintf("%s:%s", imageTarFile, buildTag),
		targetImage,
	)
}

// resolveImageFiles locates or downloads image tar and metadata files.
func (r *Runner) resolveImageFiles(ctx context.Context) (imageTarFile, imageMetadataFile string, err error) {
	imageFilePath := r.opts.ImageFilePath
	if imageFilePath == "" {
		imageFilePath, err = os.Getwd()
		if err != nil {
			return "", "", fmt.Errorf("failed to get working directory: %w", err)
		}
	}

	// If SAS URLs are provided, download files
	if r.opts.ImageTarSAS != "" {
		r.logger.Info("Downloading image tar from SAS URL")
		imageTarFile = filepath.Join(imageFilePath, r.opts.ImageTarFileName)
		if err := r.downloadFromSAS(ctx, r.opts.ImageTarSAS, imageTarFile); err != nil {
			return "", "", fmt.Errorf("failed to download image tar: %w", err)
		}
	} else {
		imageTarFile = filepath.Join(imageFilePath, r.opts.ImageTarFileName)
	}

	if r.opts.ImageMetadataSAS != "" {
		r.logger.Info("Downloading image metadata from SAS URL")
		imageMetadataFile = filepath.Join(imageFilePath, r.opts.ImageMetadataFileName)
		if err := r.downloadFromSAS(ctx, r.opts.ImageMetadataSAS, imageMetadataFile); err != nil {
			return "", "", fmt.Errorf("failed to download image metadata: %w", err)
		}
	} else {
		imageMetadataFile = filepath.Join(imageFilePath, r.opts.ImageMetadataFileName)
	}

	// Validate files exist
	if _, err := os.Stat(imageTarFile); err != nil {
		return "", "", fmt.Errorf("image tar file %s does not exist at path %s: %w", r.opts.ImageTarFileName, imageFilePath, err)
	}
	if _, err := os.Stat(imageMetadataFile); err != nil {
		return "", "", fmt.Errorf("image metadata file %s does not exist at path %s: %w", r.opts.ImageMetadataFileName, imageFilePath, err)
	}

	return imageTarFile, imageMetadataFile, nil
}

// downloadFromSAS downloads a file from a SAS URL with retry.
func (r *Runner) downloadFromSAS(ctx context.Context, sasURL, destPath string) error {
	if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
		return fmt.Errorf("failed to create directory for %s: %w", destPath, err)
	}

	return retry(ctx, maxRetries, func() error {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, sasURL, nil)
		if err != nil {
			return fmt.Errorf("failed to create HTTP request: %w", err)
		}

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return fmt.Errorf("failed to download from SAS URL: %w", err)
		}
		defer func() { _ = resp.Body.Close() }()

		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("unexpected status code %d downloading from SAS URL", resp.StatusCode)
		}

		f, err := os.Create(destPath)
		if err != nil {
			return fmt.Errorf("failed to create destination file %s: %w", destPath, err)
		}
		defer func() { _ = f.Close() }()

		if _, err := io.Copy(f, resp.Body); err != nil {
			return fmt.Errorf("failed to write file %s: %w", destPath, err)
		}

		return nil
	})
}

// readBuildTag reads the build_tag field from the image metadata JSON file.
func (r *Runner) readBuildTag(metadataFile string) (string, error) {
	data, err := os.ReadFile(metadataFile)
	if err != nil {
		return "", fmt.Errorf("failed to read metadata file: %w", err)
	}
	var metadata struct {
		BuildTag string `json:"build_tag"`
	}
	if err := json.Unmarshal(data, &metadata); err != nil {
		return "", fmt.Errorf("failed to parse metadata file: %w", err)
	}
	if metadata.BuildTag == "" {
		return "", fmt.Errorf("build_tag not found in %s", metadataFile)
	}
	return metadata.BuildTag, nil
}

// acrLogin performs az acr login with retry, returning the login server and access token.
func (r *Runner) acrLogin(ctx context.Context) (loginServer, accessToken string, err error) {
	var output []byte
	err = retry(ctx, maxRetries, func() error {
		cmd := exec.CommandContext(ctx, "az", "acr", "login",
			"--name", r.opts.TargetACR,
			"--expose-token",
			"--only-show-errors",
			"--output", "json",
		)
		var execErr error
		output, execErr = cmd.Output()
		if execErr != nil {
			var exitErr *exec.ExitError
			if errors.As(execErr, &exitErr) {
				return fmt.Errorf("az acr login failed: %s", string(exitErr.Stderr))
			}
			return fmt.Errorf("az acr login failed: %w", execErr)
		}
		return nil
	})
	if err != nil {
		return "", "", err
	}

	var response struct {
		LoginServer string `json:"loginServer"`
		AccessToken string `json:"accessToken"`
	}
	if err := json.Unmarshal(output, &response); err != nil {
		return "", "", fmt.Errorf("failed to parse ACR login response: %w", err)
	}

	return response.LoginServer, response.AccessToken, nil
}

// orasLogin performs oras login to the target registry.
func (r *Runner) orasLogin(ctx context.Context, loginServer, accessToken, registryConfig string) error {
	args := []string{"login", loginServer,
		"--username", orasUsername,
		"--password-stdin",
	}
	if registryConfig != "" {
		args = append(args, "--registry-config", registryConfig)
	}

	cmd := exec.CommandContext(ctx, "oras", args...)
	cmd.Stdin = strings.NewReader(accessToken)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// getACRDomainSuffix returns the ACR domain suffix for the current cloud.
func (r *Runner) getACRDomainSuffix(ctx context.Context) (string, error) {
	cmd := exec.CommandContext(ctx, "az", "cloud", "show",
		"--query", "suffixes.acrLoginServerEndpoint",
		"--output", "tsv",
	)
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get ACR domain suffix: %w", err)
	}
	return strings.TrimSpace(string(output)), nil
}

// runCommand runs an external command, streaming stdout/stderr.
func (r *Runner) runCommand(ctx context.Context, name string, args ...string) error {
	r.logger.V(1).Info("Running command", "cmd", name, "args", args)
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// retry executes fn with exponential backoff.
func retry(ctx context.Context, maxAttempts int, fn func() error) error {
	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		lastErr = fn()
		if lastErr == nil {
			return nil
		}
		if attempt < maxAttempts {
			delay := time.Duration(math.Pow(2, float64(attempt))) * time.Second
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(delay):
			}
		}
	}
	return fmt.Errorf("command failed after %d attempts: %w", maxAttempts, lastErr)
}
