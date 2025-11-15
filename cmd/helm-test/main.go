package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/dusted-go/logging/prettylog"
	"github.com/go-logr/logr"

	"github.com/Azure/ARO-Tools/pkg/helm"
)

func createLogger(verbosity int) logr.Logger {
	prettyHandler := prettylog.NewHandler(&slog.HandlerOptions{
		Level:       slog.Level(verbosity * -1),
		AddSource:   false,
		ReplaceAttr: nil,
	})
	return logr.FromSlogHandler(prettyHandler)
}

func main() {
	var logVerbosity = 4 // Set verbosity level (0=info, 1=debug, 2=trace)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Create logger and add to context
	logger := createLogger(logVerbosity)
	ctx = logr.NewContext(ctx, logger)

	// HARDCODED VALUES - Change these to match your setup
	opts := &helm.RawOptions{
		ReleaseName:       "credential-refresher",
		ReleaseNamespace:  "msi-credential-refresher",
		ChartDir:          "/home/abbyduke/sdp-pipelines/hcp/msi/credentialrefresher/helm", // Fixed: point to directory, not Chart.yaml file
		ValuesFile:        "/home/abbyduke/ARO-Tools/minimal-values.yaml",                  // Use the minimal test values
		KubeconfigFile:    "/home/abbyduke/.kube/config",                                   // Change this to your kubeconfig
		Timeout:           5 * time.Second,
		DryRun:            false, // Set back to true for safer testing
		RollbackOnFailure: true,

		// Optional Kusto parameters - uncomment and set if you want diagnostics
		KustoCluster:  "aroint.eastus",
		KustoDatabase: "HCPServiceLogs",
		KustoTable:    "kubesystem",
	}

	fmt.Printf("=== Helm Deployment Configuration ===\n")

	// Print configuration as JSON for better readability
	configData := map[string]interface{}{
		"releaseName":       opts.ReleaseName,
		"namespace":         opts.ReleaseNamespace,
		"chartDir":          opts.ChartDir,
		"valuesFile":        opts.ValuesFile,
		"kubeconfig":        opts.KubeconfigFile,
		"dryRun":            opts.DryRun,
		"timeout":           opts.Timeout.String(),
		"rollbackOnFailure": opts.RollbackOnFailure,
		"kusto": map[string]string{
			"cluster":  opts.KustoCluster,
			"database": opts.KustoDatabase,
			"table":    opts.KustoTable,
		},
	}

	configJSON, err := json.MarshalIndent(configData, "", "  ")
	if err != nil {
		log.Printf("Failed to marshal config: %v", err)
	} else {
		fmt.Printf("%s\n\n", string(configJSON))
	}

	// Validate options
	validated, err := opts.Validate()
	if err != nil {
		log.Fatalf("Validation failed: %v", err)
	}

	// Complete the options (load kubeconfig, chart, etc.)
	completed, err := validated.Complete()
	if err != nil {
		log.Fatalf("Completion failed: %v", err)
	}

	// Deploy!
	fmt.Println("=== Starting Helm Deployment ===")
	fmt.Println("Log output:")
	fmt.Println(strings.Repeat("-", 50))

	if err := completed.Deploy(ctx); err != nil {
		fmt.Println(strings.Repeat("-", 50))
		log.Fatalf("Deploy failed: %v", err)
	}

	fmt.Println(strings.Repeat("-", 50))
	fmt.Println("Deployment completed successfully!")
}
