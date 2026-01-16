package client

import (
	"context"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/go-logr/logr/testr"
)

func TestBuildODataFilter(t *testing.T) {
	logger := testr.New(t)
	ctx := logr.NewContext(context.Background(), logger)

	opts := &ReleaseClient{
		filter: &Filter{
			environment:      IntEnv,
			serviceGroupBase: "Microsoft.Azure.ARO.HCP",
			since:            time.Date(2025, 10, 16, 0, 0, 0, 0, time.UTC),
			until:            time.Date(2025, 10, 31, 0, 0, 0, 0, time.UTC),
		},
	}
	got, err := opts.filter.buildODataFilter(ctx, "releases")
	if err != nil {
		t.Fatalf("buildODataFilter returned error: %v", err)
	}

	want := "@container='releases' AND " +
		"\"environment\"='int' AND " +
		"\"serviceGroupBase\"='Microsoft.Azure.ARO.HCP' AND " +
		"\"timestamp\">='2025-10-16T00:00:00Z' AND " +
		"\"timestamp\"<'2025-10-31T00:00:00Z' AND " +
		"\"serviceGroup\">=''"

	if got != want {
		t.Fatalf("buildODataFilter() = %q, want %q", got, want)
	}
}
