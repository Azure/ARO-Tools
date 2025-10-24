package cmdutils

import (
	"os"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
)

// GetAzureTokenCredentials returns an Azure TokenCredential.
// If running in GitHub Actions, it uses AzureCLICredential.
// Otherwise, it uses DefaultAzureCredential.
// This allows for the azure-cli to update the token expiry of the
// workload identity token used in the GitHub action as it currently
// does not refresh: https://github.com/Azure/azure-cli/issues/28708
func GetAzureTokenCredentials() (azcore.TokenCredential, error) {
	if _, ok := os.LookupEnv("GITHUB_ACTIONS"); ok {
		return azidentity.NewAzureCLICredential(nil)
	}

	return azidentity.NewDefaultAzureCredential(nil)
}
