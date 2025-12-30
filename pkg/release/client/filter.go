package client

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/go-logr/logr"
)

const (
	DefaultServiceGroupBase = "Microsoft.Azure.ARO.HCP"
	DefaultLookback         = 7 * 24 * time.Hour
)

type Filter struct {
	environment      Environment
	since            time.Time
	until            time.Time
	serviceGroupBase string
	revision         string
	sourceRevision   string
}

func WithEnvironment(environment Environment) func(*Filter) {
	return func(opts *Filter) {
		opts.environment = environment
	}
}

func WithSince(since time.Time) func(*Filter) {
	return func(opts *Filter) {
		opts.since = since
	}
}

func WithUntil(until time.Time) func(*Filter) {
	return func(opts *Filter) {
		opts.until = until
	}
}

func WithServiceGroupBase(serviceGroupBase string) func(*Filter) {
	return func(opts *Filter) {
		opts.serviceGroupBase = serviceGroupBase
	}
}

func WithRevision(revision string) func(*Filter) {
	return func(opts *Filter) {
		opts.revision = revision
	}
}

func WithSourceRevision(sourceRevision string) func(*Filter) {

	return func(opts *Filter) {
		opts.sourceRevision = sourceRevision
	}
}

func NewFilter(options ...func(*Filter)) *Filter {
	now := time.Now().UTC()
	opts := &Filter{
		environment:      ProdEnv,
		since:            now.Add(-1 * DefaultLookback),
		until:            now,
		serviceGroupBase: DefaultServiceGroupBase,
	}
	for _, option := range options {
		option(opts)
	}

	return opts
}

func (opts *Filter) Validate() error {
	if opts.environment == "" {
		return fmt.Errorf("environment must be provided")
	}
	if opts.serviceGroupBase == "" {
		return fmt.Errorf("service group base must be provided")
	}
	if opts.since.IsZero() || opts.until.IsZero() {
		return fmt.Errorf("since and until must be provided")
	}
	if opts.since.After(opts.until) {
		return fmt.Errorf("since must be before until")
	}
	return nil
}

func (opts *Filter) buildODataFilter(ctx context.Context, containerName string) (string, error) {
	logger, err := logr.FromContext(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to get logger: %w", err)
	}

	// Build OData filter
	// Format: @container='releases' AND "timestamp" => '2025-10-16T00:00:00Z' AND "timestamp" < '2025-10-31T00:00:00Z' AND "environment"='int' AND "serviceGroupBase"='Microsoft.Azure.ARO.HCP' AND "serviceGroup" >= ''
	// The serviceGroup >= '' condition is always true, but including it causes Azure to return that tag in the response
	filters := []struct {
		key      string
		value    string
		operator string
		enabled  bool
	}{
		{key: "environment", value: string(opts.environment), operator: "=", enabled: true},
		{key: "serviceGroupBase", value: opts.serviceGroupBase, operator: "=", enabled: true},
		{key: "timestamp", value: opts.since.Format(time.RFC3339), operator: ">=", enabled: true},
		{key: "timestamp", value: opts.until.Format(time.RFC3339), operator: "<", enabled: true},
		{key: "serviceGroup", value: "", operator: ">=", enabled: true},
		{key: "revision", value: opts.revision, operator: "=", enabled: opts.revision != ""},
		{key: "upstreamRevision", value: opts.sourceRevision, operator: "=", enabled: opts.sourceRevision != ""},
	}

	filter := make([]string, 0, len(filters))
	filter = append(filter, fmt.Sprintf("@container='%s'", containerName))
	for _, item := range filters {
		if item.enabled {
			filter = append(filter, fmt.Sprintf("\"%s\"%s'%s'", item.key, item.operator, item.value))
		}
	}

	logger.V(1).Info("filter", "filter", strings.Join(filter, " AND "))
	return strings.Join(filter, " AND "), nil
}
