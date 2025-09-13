package helm

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/go-logr/logr"
	"github.com/spf13/cobra"
	"helm.sh/helm/v4/pkg/action"
	chartv2 "helm.sh/helm/v4/pkg/chart/v2"
	"helm.sh/helm/v4/pkg/chart/v2/loader"
	"helm.sh/helm/v4/pkg/kube"
	helmreleasev1 "helm.sh/helm/v4/pkg/release/v1"
	"helm.sh/helm/v4/pkg/storage/driver"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	yamlutil "k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	corev1applyconfigurations "k8s.io/client-go/applyconfigurations/core/v1"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	corev1client "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/utils/ptr"

	"sigs.k8s.io/yaml"

	"github.com/Azure/ARO-Tools/pkg/cmdutils"
)

func DefaultOptions() *RawOptions {
	return &RawOptions{
		Timeout: 5 * time.Minute,
	}
}

func BindOptions(opts *RawOptions, cmd *cobra.Command) error {
	cmd.Flags().StringArrayVar(&opts.NamespaceFiles, "namespace-file", opts.NamespaceFiles, "Path to a namespace manifest to deploy before the Helm release.")

	cmd.Flags().StringVar(&opts.ReleaseName, "release-name", opts.ReleaseName, "Name of the Helm release being deployed.")
	cmd.Flags().StringVar(&opts.ReleaseNamespace, "release-namespace", opts.ReleaseNamespace, "Namespace in which the Helm release is deployed. Will create a basic namespace manifest unless a more complex namespace configuration is provided as a file.")
	cmd.Flags().StringVar(&opts.ChartDir, "chart-dir", opts.ChartDir, "Path to the directory containing the Helm chart.")
	cmd.Flags().StringVar(&opts.ValuesFile, "values-file", opts.ValuesFile, "Path to the Helm values file.")

	cmd.Flags().DurationVar(&opts.Timeout, "timeout", opts.Timeout, "Timeout for waiting on the Helm release.")

	cmd.Flags().StringVar(&opts.KubeconfigFile, "kubeconfig", opts.KubeconfigFile, "Path to the kubeconfig.")
	cmd.Flags().BoolVar(&opts.DryRun, "dry-run", opts.DryRun, "Do not make any changes to the Kubernetes API server.")

	return nil
}

// RawOptions holds input values.
type RawOptions struct {
	NamespaceFiles []string

	ReleaseName      string
	ReleaseNamespace string
	ChartDir         string
	ValuesFile       string

	Timeout time.Duration

	KubeconfigFile string
	DryRun         bool
}

// validatedOptions is a private wrapper that enforces a call of Validate() before Complete() can be invoked.
type validatedOptions struct {
	*RawOptions
	*cmdutils.ValidatedOptions
}

type ValidatedOptions struct {
	// Embed a private pointer that cannot be instantiated outside of this package.
	*validatedOptions
}

// completedOptions is a private wrapper that enforces a call of Complete() before Config generation can be invoked.
type completedOptions struct {
	Namespaces       []corev1.Namespace
	NamespacesClient corev1client.NamespaceInterface

	DynamicClient *dynamic.DynamicClient
	RESTMapper    meta.RESTMapper

	ActionConfig *action.Configuration

	ReleaseName      string
	ReleaseNamespace string
	Chart            *chartv2.Chart
	Values           map[string]any

	Timeout time.Duration
	DryRun  bool
}

type Options struct {
	// Embed a private pointer that cannot be instantiated outside of this package.
	*completedOptions
}

func (o *RawOptions) Validate() (*ValidatedOptions, error) {
	for _, item := range []struct {
		flag  string
		name  string
		value *string
	}{
		{flag: "release-name", name: "Helm release name", value: &o.ReleaseName},
		{flag: "release-namespace", name: "Helm release namespace", value: &o.ReleaseNamespace},
		{flag: "chart-dir", name: "Helm chart directory", value: &o.ChartDir},
		{flag: "values-file", name: "Helm values file", value: &o.ValuesFile},
		{flag: "kubeconfig", name: "Kubeconfig file", value: &o.KubeconfigFile},
	} {
		if item.value == nil || *item.value == "" {
			return nil, fmt.Errorf("the %s must be provided with --%s", item.name, item.flag)
		}
	}

	return &ValidatedOptions{
		validatedOptions: &validatedOptions{
			RawOptions: o,
		},
	}, nil
}

func (o *ValidatedOptions) Complete() (*Options, error) {
	rawConfig, err := clientcmd.LoadFromFile(o.KubeconfigFile)
	if err != nil {
		return nil, fmt.Errorf("failed to load kubeconfig: %w", err)
	}

	cfg, err := clientcmd.NewNonInteractiveClientConfig(*rawConfig, rawConfig.CurrentContext, nil, nil).ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to create client config: %w", err)
	}

	// Create the Kubernetes clients
	dynamicClient, err := dynamic.NewForConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create dynamic client: %w", err)
	}

	clientset, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create Kubernetes client: %w", err)
	}

	var foundReleaseNamespace bool
	var namespaces []corev1.Namespace
	for _, file := range o.NamespaceFiles {
		raw, err := os.ReadFile(file)
		if err != nil {
			return nil, fmt.Errorf("failed to read namespace %s: %w", file, err)
		}
		ns := corev1.Namespace{}
		if err := yaml.Unmarshal(raw, &ns); err != nil {
			return nil, fmt.Errorf("failed to unmarshal namespace %s: %w", file, err)
		}
		foundReleaseNamespace = foundReleaseNamespace || ns.Name == o.ReleaseName
		namespaces = append(namespaces, ns)
	}
	if !foundReleaseNamespace {
		// if the user hasn't provided an explicit manifest for the release namespace, let's add a minimal one
		namespaces = append(namespaces, corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: o.ReleaseName}})
	}

	actionCfg := &action.Configuration{}
	cliOpts := &genericclioptions.ConfigFlags{
		KubeConfig: ptr.To(o.KubeconfigFile),
		Namespace:  ptr.To(o.ReleaseNamespace),
	}
	if err := actionCfg.Init(cliOpts, o.ReleaseNamespace, ""); err != nil {
		return nil, err
	}
	restMapper, err := cliOpts.ToRESTMapper()
	if err != nil {
		return nil, fmt.Errorf("failed to create RESTMapper: %w", err)
	}

	chartPath, err := filepath.Abs(o.ChartDir)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve chart directory %s: %w", o.ChartDir, err)
	}

	chart, err := loader.Load(chartPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load chart %s: %w", o.ChartDir, err)
	}

	rawValues, err := os.ReadFile(o.ValuesFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read values file %s: %w", o.ValuesFile, err)
	}

	values := map[string]interface{}{}
	if err := yaml.Unmarshal(rawValues, &values); err != nil {
		return nil, fmt.Errorf("failed to unmarshal values file %s: %w", o.ValuesFile, err)
	}

	return &Options{
		completedOptions: &completedOptions{
			Namespaces:       namespaces,
			NamespacesClient: clientset.CoreV1().Namespaces(),

			DynamicClient: dynamicClient,
			RESTMapper:    restMapper,

			ActionConfig: actionCfg,

			ReleaseName:      o.ReleaseName,
			ReleaseNamespace: o.ReleaseNamespace,

			Chart:  chart,
			Values: values,

			Timeout: o.Timeout,
			DryRun:  o.DryRun,
		},
	}, nil
}

func (opts *Options) Deploy(ctx context.Context) error {
	logger, err := logr.FromContext(ctx)
	if err != nil {
		return fmt.Errorf("failed to create logger: %w", err)
	}

	logger.Info("Applying namespaces.")
	// Helm does not let us manage namespaces easily, so we need to apply them ourselves, up-front.
	for _, namespace := range opts.Namespaces {
		if err := applyNamespace(ctx, logger, opts.NamespacesClient, namespace, opts.DryRun); err != nil {
			return err
		}
	}

	logger.Info("Rolling out Helm release.", "dryRun", opts.DryRun)
	release, releaseErr := runHelmUpgrade(ctx, logger, opts)
	if releaseErr != nil {
		logger.Error(releaseErr, "Failed to roll out the Helm release.")
	}
	logger.Info("Finished deploying Helm release.")

	if opts.DryRun {
		if releaseErr != nil {
			return fmt.Errorf("failed to create a dry-run Helm release: %w", err)
		}
		logger.Info("Validating Helm release contents for dry-run.")
		if err := validateHelmResources(ctx, logger, opts, release); err != nil {
			return fmt.Errorf("failed to validate Helm release contents for dry-run: %w", err)
		}
		logger.Info("Finished validating Helm release contents.")
	} else {
		logger.Info("Running inline diagnostics.")
		if err := runDiagnostics(ctx, logger, opts); err != nil {
			return fmt.Errorf("capturing diagnostics failed: %w", err)
		}
	}

	logger.Info("Deployment complete.")
	return releaseErr
}

func applyNamespace(ctx context.Context, logger logr.Logger, client corev1client.NamespaceInterface, namespace corev1.Namespace, dryRun bool) error {
	cfg := corev1applyconfigurations.Namespace(namespace.Name)
	if len(namespace.Labels) > 0 {
		cfg = cfg.WithLabels(namespace.Labels)
	}
	if len(namespace.Annotations) > 0 {
		cfg = cfg.WithAnnotations(namespace.Annotations)
	}
	if len(namespace.Spec.Finalizers) > 0 {
		cfg = cfg.WithSpec(corev1applyconfigurations.NamespaceSpec().WithFinalizers(namespace.Spec.Finalizers...))
	}

	var dryRunOpt []string
	if dryRun {
		dryRunOpt = append(dryRunOpt, "All")
	}
	if _, err := client.Apply(ctx, cfg, metav1.ApplyOptions{FieldManager: "helm", DryRun: dryRunOpt}); err != nil {
		return fmt.Errorf("failed to apply namespace %s: %w", namespace.Name, err)
	}
	logger.WithValues("namespace", namespace.Name).Info("Namespace applied successfully.")
	return nil
}

// runHelmUpgrade effectively re-packages `helm upgrade --install` by mirroring the logic here:
// https://github.com/helm/helm/blob/e2cbc5c0c94e6a12473fb7d1a7a232498aa4cda6/pkg/cmd/upgrade.go#L102
// This helps us over using exec.Command as:
//   - our deployer is self-contained and easy to use
//   - we get to control the version of Helm used to deploy directly, as opposed to being vulnerable to whatever the Ev2
//     or local environment we run in happens to use
func runHelmUpgrade(ctx context.Context, logger logr.Logger, opts *Options) (*helmreleasev1.Release, error) {
	logger.Info("Searching for release history...")
	historyClient := action.NewHistory(opts.ActionConfig)
	historyClient.Max = 1
	versions, err := historyClient.Run(opts.ReleaseName)
	// If a release does not exist, install it.
	if err == driver.ErrReleaseNotFound || isReleaseUninstalled(versions) {
		logger.Info("No release history found, running the first release...")
		installClient := action.NewInstall(opts.ActionConfig)
		installClient.WaitStrategy = kube.StatusWatcherStrategy
		installClient.Namespace = opts.ReleaseNamespace
		installClient.Timeout = opts.Timeout
		installClient.ServerSideApply = true

		if opts.DryRun {
			installClient.DryRun = true
			installClient.DryRunOption = "server"
			installClient.HideSecret = true
		}

		return installClient.RunWithContext(ctx, opts.Chart, opts.Values)
	}
	logger.Info("Found release history, upgrading...", "numVersions", len(versions))

	upgradeClient := action.NewUpgrade(opts.ActionConfig)
	upgradeClient.WaitStrategy = kube.StatusWatcherStrategy
	upgradeClient.Namespace = opts.ReleaseNamespace
	upgradeClient.Timeout = opts.Timeout
	upgradeClient.Install = true
	upgradeClient.ServerSideApply = "true"

	if opts.DryRun {
		upgradeClient.DryRun = true
		upgradeClient.DryRunOption = "server"
		upgradeClient.HideSecret = true
	}

	return upgradeClient.RunWithContext(ctx, opts.ReleaseName, opts.Chart, opts.Values)
}

func isReleaseUninstalled(versions []*helmreleasev1.Release) bool {
	return len(versions) > 0 && versions[len(versions)-1].Info.Status == helmreleasev1.StatusUninstalled
}

func runDiagnostics(ctx context.Context, logger logr.Logger, opts *Options) error {
	statusClient := action.NewStatus(opts.ActionConfig)
	release, err := statusClient.Run(opts.ReleaseName)
	if err != nil {
		return fmt.Errorf("failed to get status for release %s: %w", opts.ReleaseName, err)
	}

	logger.Info(
		"Determined release status.",
		"release", release.Name,
		"namespace", release.Namespace,
		"status", release.Info.Status,
		"description", release.Info.Description,
		"values", release.Config,
	)

	// TODO: add in Kusto deep-links
	// TOOD: do we still want/need to dump the YAMLs of the resources that were just created?

	return nil
}

func validateHelmResources(ctx context.Context, logger logr.Logger, opts *Options, release *helmreleasev1.Release) error {
	logger.Info("Validating objects in release manifest.")
	inputDecoder := yamlutil.NewYAMLOrJSONDecoder(bytes.NewBuffer([]byte(release.Manifest)), 4096)
	failed := false
	for {
		ext := runtime.RawExtension{}
		if err := inputDecoder.Decode(&ext); err != nil {
			if err == io.EOF {
				break
			}
			return fmt.Errorf("failed to parse release manifest: %v", err)
		}
		ext.Raw = bytes.TrimSpace(ext.Raw)
		if len(ext.Raw) == 0 || bytes.Equal(ext.Raw, []byte("null")) {
			continue
		}

		obj := &unstructured.Unstructured{}
		if err := yaml.Unmarshal(ext.Raw, obj); err != nil {
			return fmt.Errorf("failed to unmarshal release manifest: %v", err)
		}

		objLogger := logger.WithValues("gvk", obj.GroupVersionKind().String(), "namespace", obj.GetNamespace(), "name", obj.GetName())
		objLogger.Info("Decoded resource from manifests.")

		mapping, err := opts.RESTMapper.RESTMapping(obj.GroupVersionKind().GroupKind(), obj.GroupVersionKind().Version)
		if err != nil {
			return fmt.Errorf("unable to determine GVR mapping for GVK %s: %v", obj.GroupVersionKind(), err)
		}

		if _, err := opts.DynamicClient.Resource(mapping.Resource).Namespace(obj.GetNamespace()).Apply(ctx, obj.GetName(), obj, metav1.ApplyOptions{
			FieldManager: "helm", DryRun: []string{"All"},
		}); err != nil {
			failed = true
			objLogger.Error(err, "Failed to validate resource using server-side dry-run.")
		} else {
			objLogger.Info("Validated resource using server-side dry-run.")
		}
	}
	if failed {
		return errors.New("failed validating release manifests")
	}
	return nil
}
