package config

import (
	"os"

	"sigs.k8s.io/yaml"
)

// SecretSync holds encrypted blobs and public keys for a number of key vaults that have external data populated into them.
type SecretSync struct {
	// KeyVaults stores the data for each key vault keyed by the connection URI, like "my-key-vault.azure-test.net".
	// As the connection URI contains a tenant-unique name and a tenant-specific domain, these are globally unique.
	KeyVaults map[string]KeyVault `yaml:"keyVaults"`
}

// KeyVault holds encrypted data that should be populated into a key vault and the public key used to encrypt these data.
type KeyVault struct {
	// EncryptedSecrets stores Azure Key Vault secret data by secret name that should be populated into the KeyVault.
	// These data are base64-encoded to be easy to embed into this text-based data format.
	EncryptedSecrets map[string]string `yaml:"encryptedSecrets"`
	// PublicKeyData holds the PKIX public key data in PEM block format.
	PublicKeyData string `yaml:"publicKeyData"`
}

func Load(path string) (*SecretSync, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var out SecretSync
	return &out, yaml.Unmarshal(raw, &out)
}
