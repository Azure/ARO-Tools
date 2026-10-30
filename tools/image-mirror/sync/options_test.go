package sync

import (
	"testing"
)

func TestRawOptions_Validate_RegistryMode(t *testing.T) {
	tests := []struct {
		name    string
		opts    RawOptions
		wantErr string
	}{
		{
			name:    "missing target ACR",
			opts:    RawOptions{},
			wantErr: "the target ACR must be provided with --target-acr",
		},
		{
			name: "missing repository",
			opts: RawOptions{
				TargetACR: "myacr",
			},
			wantErr: "the repository must be provided with --repository",
		},
		{
			name: "missing source registry",
			opts: RawOptions{
				TargetACR:  "myacr",
				Repository: "myrepo",
			},
			wantErr: "the source registry must be provided with --source-registry for registry mode",
		},
		{
			name: "missing digest",
			opts: RawOptions{
				TargetACR:      "myacr",
				Repository:     "myrepo",
				SourceRegistry: "source.azurecr.io",
			},
			wantErr: "the digest must be provided with --digest for registry mode",
		},
		{
			name: "missing pull secret KV",
			opts: RawOptions{
				TargetACR:      "myacr",
				Repository:     "myrepo",
				SourceRegistry: "source.azurecr.io",
				Digest:         "sha256:abc123",
			},
			wantErr: "the pull secret KeyVault must be provided with --pull-secret-kv for registry mode",
		},
		{
			name: "missing pull secret name",
			opts: RawOptions{
				TargetACR:      "myacr",
				Repository:     "myrepo",
				SourceRegistry: "source.azurecr.io",
				Digest:         "sha256:abc123",
				PullSecretKV:   "mykeyvault",
			},
			wantErr: "the pull secret name must be provided with --pull-secret for registry mode",
		},
		{
			name: "valid registry mode options",
			opts: RawOptions{
				TargetACR:      "myacr",
				Repository:     "myrepo",
				SourceRegistry: "source.azurecr.io",
				Digest:         "sha256:abc123",
				PullSecretKV:   "mykeyvault",
				PullSecretName: "mysecret",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := tt.opts.Validate()
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error %q, got nil", tt.wantErr)
				}
				if err.Error() != tt.wantErr {
					t.Fatalf("expected error %q, got %q", tt.wantErr, err.Error())
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
			}
		})
	}
}

func TestRawOptions_Validate_OCILayoutMode(t *testing.T) {
	tests := []struct {
		name    string
		opts    RawOptions
		wantErr string
	}{
		{
			name: "missing image tar file name",
			opts: RawOptions{
				TargetACR:  "myacr",
				Repository: "myrepo",
				CopyFrom:   "oci-layout",
			},
			wantErr: "the image tar file name must be provided with --image-tar-file for oci-layout mode",
		},
		{
			name: "missing image metadata file name",
			opts: RawOptions{
				TargetACR:        "myacr",
				Repository:       "myrepo",
				CopyFrom:         "oci-layout",
				ImageTarFileName: "image.tar",
			},
			wantErr: "the image metadata file name must be provided with --image-metadata-file for oci-layout mode",
		},
		{
			name: "valid oci-layout mode options",
			opts: RawOptions{
				TargetACR:             "myacr",
				Repository:            "myrepo",
				CopyFrom:              "oci-layout",
				ImageTarFileName:      "image.tar",
				ImageMetadataFileName: "metadata.json",
			},
		},
		{
			name: "valid oci-layout mode with SAS URLs",
			opts: RawOptions{
				TargetACR:             "myacr",
				Repository:            "myrepo",
				CopyFrom:              "oci-layout",
				ImageTarFileName:      "image.tar",
				ImageMetadataFileName: "metadata.json",
				ImageTarSAS:           "https://storage.blob.core.windows.net/container/image.tar?sig=abc",
				ImageMetadataSAS:      "https://storage.blob.core.windows.net/container/metadata.json?sig=abc",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := tt.opts.Validate()
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error %q, got nil", tt.wantErr)
				}
				if err.Error() != tt.wantErr {
					t.Fatalf("expected error %q, got %q", tt.wantErr, err.Error())
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
			}
		})
	}
}

func TestValidatedOptions_Complete(t *testing.T) {
	raw := &RawOptions{
		TargetACR:      "myacr",
		Repository:     "myrepo",
		SourceRegistry: "source.azurecr.io",
		Digest:         "sha256:abc123",
		PullSecretKV:   "mykeyvault",
		PullSecretName: "mysecret",
		DryRun:         true,
	}
	validated, err := raw.Validate()
	if err != nil {
		t.Fatalf("unexpected validation error: %v", err)
	}
	completed, err := validated.Complete()
	if err != nil {
		t.Fatalf("unexpected completion error: %v", err)
	}
	if completed.TargetACR != "myacr" {
		t.Errorf("expected TargetACR 'myacr', got %q", completed.TargetACR)
	}
	if completed.Repository != "myrepo" {
		t.Errorf("expected Repository 'myrepo', got %q", completed.Repository)
	}
	if completed.DryRun != true {
		t.Errorf("expected DryRun true, got false")
	}
}
