package client

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/go-logr/logr"
	"github.com/stoewer/go-strcase"
	"gopkg.in/yaml.v3"

	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob/service"
)

const (
	DefaultStorageAccountURL = "https://aroreleases.blob.core.windows.net/"
	DefaultStorageContainer  = "releases"
	ReleaseFileName          = "release.yaml"
	ConfigFileName           = "config.yaml"
	DefaultLimit             = 0
)

const (
	timestampTagKey = "timestamp"
)

type ReleaseClient struct {
	serviceClient        *service.Client
	storageContainerName string
	includeComponents    bool
	limit                int
	filter               *Filter
	warningSink          func(error)
}

func WithServiceClient(serviceClient *service.Client) func(*ReleaseClient) {
	return func(opts *ReleaseClient) {
		opts.serviceClient = serviceClient
	}
}

func WithStorageContainerName(storageContainerName string) func(*ReleaseClient) {
	return func(opts *ReleaseClient) {
		opts.storageContainerName = storageContainerName
	}
}

func WithIncludeComponents(includeComponents bool) func(*ReleaseClient) {
	return func(opts *ReleaseClient) {
		opts.includeComponents = includeComponents
	}
}

func WithLimit(limit int) func(*ReleaseClient) {

	return func(opts *ReleaseClient) {
		opts.limit = limit
	}
}

func WithWarningSink(sink func(error)) func(*ReleaseClient) {
	return func(opts *ReleaseClient) {
		opts.warningSink = sink
	}
}

func NewOptions(filter *Filter, options ...func(*ReleaseClient)) *ReleaseClient {
	opts := &ReleaseClient{
		serviceClient:        nil,
		storageContainerName: DefaultStorageContainer,
		includeComponents:    false,
		limit:                DefaultLimit,
		filter:               filter,
		warningSink:          nil,
	}
	for _, option := range options {
		option(opts)
	}
	return opts
}

func (opts *ReleaseClient) Validate() error {
	if opts.filter == nil {
		return fmt.Errorf("filter must be provided")
	}
	if err := opts.filter.Validate(); err != nil {
		return fmt.Errorf("failed to validate filter: %w", err)
	}
	if opts.limit < 0 {
		return fmt.Errorf("limit must be greater than or equal to 0")
	}
	if opts.serviceClient == nil {
		return fmt.Errorf("service client must be provided")
	}
	if opts.storageContainerName == "" {
		return fmt.Errorf("storage container name must be provided")
	}
	return nil
}

func (opts *ReleaseClient) ListReleaseDeployments(ctx context.Context) ([]*ReleaseDeployment, error) {
	logger, err := logr.FromContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get logger: %w", err)
	}

	if err := opts.Validate(); err != nil {
		return nil, fmt.Errorf("failed to validate list options: %w", err)
	}

	tagFilter, err := opts.filter.buildODataFilter(ctx, opts.storageContainerName)
	if err != nil {
		return nil, fmt.Errorf("failed to build filter: %w", err)
	}

	type blobWithTime struct {
		name      string
		timestamp time.Time
	}

	var blobs []blobWithTime
	var marker *string
	for {
		resp, err := opts.serviceClient.FilterBlobs(ctx, tagFilter, &service.FilterBlobsOptions{
			Marker: marker,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to filter blobs: %w", err)
		}

		if resp.Blobs != nil {
			for _, blob := range resp.Blobs {
				if !strings.HasSuffix(*blob.Name, "/"+ReleaseFileName) {
					continue
				}

				var timestampStr string
				if blob.Tags != nil {
					for _, tag := range blob.Tags.BlobTagSet {
						if *tag.Key == timestampTagKey {
							timestampStr = *tag.Value
							break
						}
					}
				}

				if timestampStr == "" {
					logger.Error(errors.New("no timestamp found for blob"), "missing timestamp tag", "blob", *blob.Name)
					continue
				}

				timestamp, err := time.Parse(time.RFC3339, timestampStr)
				if err != nil {
					logger.Error(err, "failed to parse timestamp", "blob", *blob.Name)
					continue
				}

				blobs = append(blobs, blobWithTime{
					name:      *blob.Name,
					timestamp: timestamp,
				})
			}
		}

		if resp.NextMarker == nil || len(*resp.NextMarker) == 0 {
			break
		}
		marker = resp.NextMarker
	}

	if len(blobs) == 0 {
		return []*ReleaseDeployment{}, nil
	}

	sort.Slice(blobs, func(i, j int) bool {
		return blobs[i].timestamp.After(blobs[j].timestamp)
	})

	if opts.limit > 0 && opts.limit < len(blobs) {
		blobs = blobs[:opts.limit]
	}

	// Download and parse each release
	deployments := make([]*ReleaseDeployment, 0, len(blobs))
	for _, blob := range blobs {
		deployment, err := opts.downloadAndParseRelease(ctx, blob.name)
		if err != nil {
			logger.Error(err, "failed to download and parse release", "blob", blob.name)
			continue
		}

		deployments = append(deployments, deployment)
	}

	return deployments, nil
}

func (opts *ReleaseClient) downloadAndParseRelease(ctx context.Context, blobName string) (*ReleaseDeployment, error) {
	logger, err := logr.FromContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get logger: %w", err)
	}

	downloadResponse, err := opts.serviceClient.NewContainerClient(opts.storageContainerName).
		NewBlobClient(blobName).DownloadStream(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to download blob: %w", err)
	}
	defer func() {
		if err := downloadResponse.Body.Close(); err != nil {
			logger.Error(err, "failed to close blob body", "blob", blobName)
		}
	}()

	content, err := io.ReadAll(downloadResponse.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read blob content: %w", err)
	}

	var deployment ReleaseDeployment
	if err := yaml.Unmarshal(content, &deployment); err != nil {
		return nil, fmt.Errorf("failed to unmarshal YAML: %w", err)
	}

	if opts.includeComponents && len(deployment.Target.RegionConfigs) > 0 {
		// We currently assume that there's only one region per target (the publishing tool writes one).
		// TODO: Process all regions once the publishing tool writes more than one.
		components, err := opts.downloadAndParseComponents(ctx, blobName, deployment.Target.RegionConfigs[0])
		if err != nil {
			return nil, fmt.Errorf("failed to download and parse components: %w", err)
		}
		deployment.Components = components
	}

	return &deployment, nil
}

func (opts *ReleaseClient) downloadAndParseComponents(ctx context.Context, releasePath, region string) (Components, error) {
	logger, err := logr.FromContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get logger: %w", err)
	}

	blobName := strings.Join([]string{filepath.Dir(releasePath), region, ConfigFileName}, "/")
	downloadResponse, err := opts.serviceClient.NewContainerClient(opts.storageContainerName).
		NewBlobClient(blobName).DownloadStream(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to download config: %w", err)
	}
	defer func() {
		if err := downloadResponse.Body.Close(); err != nil {
			logger.Error(err, "failed to close blob body", "blob", blobName)
		}
	}()

	content, err := io.ReadAll(downloadResponse.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read config: %w", err)
	}

	components, warnings, err := parseComponentsYAML(logger, content)
	if err != nil {
		return nil, err
	}
	for _, w := range warnings {
		if opts.warningSink != nil {
			opts.warningSink(w)
		} else {
			logger.V(0).Info("components parse warning", "warning", w.Error(), "blob", blobName)
		}
	}
	return components, nil
}

func parseComponentsYAML(logger logr.Logger, content []byte) (Components, []error, error) {
	var root yaml.Node
	if err := yaml.Unmarshal(content, &root); err != nil {
		return nil, nil, fmt.Errorf("failed to parse YAML: %w", err)
	}

	if len(root.Content) == 0 {
		return nil, nil, fmt.Errorf("parsed YAML document is empty")
	}

	components := Components{}
	warnings := []error{}

	// the YAML document with the list of components is already a
	// projection of the config file with just the interesting values
	// in it -- the component's images. Just walk the yaml
	// tree and extract all the scalar nodes.
	var walk func(n *yaml.Node, path []string)
	walk = func(n *yaml.Node, path []string) {
		switch n.Kind {
		case yaml.MappingNode:
			for i := 0; i < len(n.Content); i += 2 {
				keyNode := n.Content[i]
				valNode := n.Content[i+1]
				walk(valNode, append(path, keyNode.Value))
			}
		case yaml.SequenceNode:
			for i, valNode := range n.Content {
				walk(valNode, append(path, strconv.Itoa(i)))
			}
		case yaml.ScalarNode:
			if n.Tag != "!!str" || n.Value == "" {
				warnings = append(warnings, fmt.Errorf("string node expected at %s (tag=%s)", strings.Join(path, "."), n.Tag))
				return
			}

			if len(path) == 0 {
				warnings = append(warnings, fmt.Errorf("unexpected path length: path is empty"))
				return
			}

			// Drop the last segment (e.g. "digest" or "sha") from the path to get the component name.
			nameParts := make([]string, 0, len(path)-1)
			for _, part := range path[:len(path)-1] {
				nameParts = append(nameParts, strcase.KebabCase(part))
			}
			components[strings.Join(nameParts, ".")] = strings.TrimPrefix(n.Value, "sha256:")
		}
	}

	walk(root.Content[0], nil)

	if len(warnings) > 0 {
		logger.V(1).Info("parsed components with warnings", "warnings", len(warnings))
	}

	return components, warnings, nil
}
