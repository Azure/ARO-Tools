module github.com/Azure/ARO-Tools/tools/release

go 1.25.0

require (
	github.com/Azure/ARO-Tools/testutil v0.0.0-20260227032723-11f678744bf9
	github.com/Azure/azure-sdk-for-go/sdk/storage/azblob v1.6.4
	github.com/go-logr/logr v1.4.4
	github.com/google/go-cmp v0.7.0
	github.com/stoewer/go-strcase v1.3.1
	gopkg.in/yaml.v3 v3.0.1
)

require (
	github.com/Azure/azure-sdk-for-go/sdk/azcore v1.23.0 // indirect
	github.com/Azure/azure-sdk-for-go/sdk/azidentity v1.14.0 // indirect
	github.com/Azure/azure-sdk-for-go/sdk/internal v1.12.0 // indirect
	github.com/stretchr/testify v1.12.0 // indirect
	go.yaml.in/yaml/v2 v2.4.3 // indirect
	golang.org/x/net v0.57.0 // indirect
	golang.org/x/text v0.40.0 // indirect
	sigs.k8s.io/yaml v1.6.0 // indirect
)
