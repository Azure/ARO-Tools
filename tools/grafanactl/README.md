# grafanactl

A command-line utility for managing Azure Managed Grafana instances, used in the ARO HCP context.

## Overview

grafanactl helps maintain Azure Managed Grafana instances by providing tools to:
- List all datasources in a Grafana instance
- Remove orphaned Azure Monitor Workspace integrations
- Clean up stale datasources pointing to deleted resources
- Sync dashboards and folders from git to Grafana

This tool is particularly useful when Azure Monitor Workspaces (Prometheus instances) are removed from your infrastructure but their references remain in Grafana, creating stale integrations.

## Installation

Build the tool from source:

```bash
go build -o grafanactl .
```

## Authentication

grafanactl uses Azure Active Directory authentication. Ensure you are logged into Azure CLI:

```bash
az login
```

The tool will use the same authentication context as other Azure CLI tools.

## Usage

### Common Flags

All commands require these basic parameters:

- `--subscription` - Azure subscription ID
- `--resource-group` - Azure resource group name
- `--grafana-name` - Azure Managed Grafana instance name
- `--output` - Output format: `table` (default) or `json`
- `-v, --verbosity` - Set logging verbosity level (0-10)

For sovereign clouds (e.g. Fairfax ), pass the ARM endpoint and AAD
authority directly. Both flags must be set together; if neither is provided,
the public Azure cloud is used. Each flag accepts either a hostname or a full
`https://` URL — bare hostnames are normalized to URL form automatically.

- `--arm-endpoint` - Azure Resource Manager endpoint (e.g.
  `management.usgovcloudapi.net` for Fairfax).
- `--aad-authority` - Microsoft Entra ID authority (e.g.
  `login.microsoftonline.us` for Fairfax).

### List Commands

#### List Datasources

Display all datasources configured in your Grafana instance:

```bash
grafanactl list datasources \
  --subscription "your-subscription-id" \
  --resource-group "your-resource-group" \
  --grafana-name "your-grafana-instance"
```

Output formats:
- **Table format** (default): Human-readable table with ID, name, type, and URL
- **JSON format**: Machine-readable JSON for scripting and integration

```bash
# JSON output for scripting
grafanactl list datasources \
  --subscription "your-subscription-id" \
  --resource-group "your-resource-group" \
  --grafana-name "your-grafana-instance" \
  --output json
```

### Clean Commands

Clean commands help maintain your Grafana instance by removing stale references and orphaned resources.

#### Clean Datasources (Azure Monitor Workspace Integrations)

Remove orphaned Azure Monitor Workspace integrations from the Grafana resource. This cleans up references to Azure Monitor Workspaces that no longer exist:

```bash
# Preview changes (dry-run)
grafanactl clean datasources \
  --subscription "your-subscription-id" \
  --resource-group "your-resource-group" \
  --grafana-name "your-grafana-instance" \
  --dry-run

# Apply changes
grafanactl clean datasources \
  --subscription "your-subscription-id" \
  --resource-group "your-resource-group" \
  --grafana-name "your-grafana-instance"
```

#### Fixup Datasources

Delete orphaned datasources within the Grafana instance itself. This removes any Managed Prometheus datasources that are no longer valid:

```bash
# Preview changes (dry-run)
grafanactl clean fixup-datasources \
  --subscription "your-subscription-id" \
  --resource-group "your-resource-group" \
  --grafana-name "your-grafana-instance" \
  --dry-run

# Apply changes
grafanactl clean fixup-datasources \
  --subscription "your-subscription-id" \
  --resource-group "your-resource-group" \
  --grafana-name "your-grafana-instance"
```

### Sync Commands

Sync commands help keep your Grafana instance in sync with dashboard definitions stored in git.

#### Sync Dashboards

Synchronize dashboards and folders from a configuration file to Grafana. This will:
- Create folders that don't exist in Grafana
- Create or update dashboards from JSON files
- Delete stale dashboards that are no longer in git (excluding Azure managed folders)
- Validate dashboards and report errors/warnings

```bash
# Preview changes (dry-run)
grafanactl sync dashboards \
  --subscription "your-subscription-id" \
  --resource-group "your-resource-group" \
  --grafana-name "your-grafana-instance" \
  --config-file "../../observability/observability.yaml" \
  --dry-run

# Apply changes
grafanactl sync dashboards \
  --subscription "your-subscription-id" \
  --resource-group "your-resource-group" \
  --grafana-name "your-grafana-instance" \
  --config-file "../../observability/observability.yaml"
```

The config file (e.g., `observability.yaml`) defines:
- `grafana-dashboards.dashboardFolders`: List of folders with `name` and `path` to dashboard JSON files
- `grafana-dashboards.azureManagedFolders`: List of folder names managed by Azure (will not be modified)

### Manage Commands

#### Reconcile Grafana

Create or update the Azure Managed Grafana instance and reconcile its Azure
Monitor Workspace integrations:

```bash
grafanactl manage reconcile \
  --subscription "your-subscription-id" \
  --resource-group "your-resource-group" \
  --grafana-name "your-grafana-instance" \
  --location "eastus"
```

ADX integration fabrics are disabled by default. When enabled, both the
environment and complete geography set must be provided explicitly:

- `--adx-integrations-enabled`: Reconcile owned
  `Microsoft.Dashboard/grafana/integrationFabrics` child resources for
  Kusto clusters tagged with `aroHCPPurpose=logs`.
- `--adx-integrations-environment`: Required alphanumeric or hyphenated
  `aroHCPEnvironment` tag value.
- `--adx-integrations-geographies`: Comma-separated, case-insensitive
  complete set of expected `aroHCPGeoShortId` values. Each geography must
  resolve to exactly one succeeded cluster.
- `--adx-integrations-scenario`: Optional scenario value.
- `--adx-integrations-target-resource-id`: Optional target resource ID.

Discovery fails closed before reading or changing integration fabrics when a
requested geography is missing, duplicated, untagged, or not fully
provisioned. Dry-run performs discovery and planning but does not create,
update, or delete child resources.

The scenario and target resource ID are Resource Provider contract inputs.
They are intentionally not defaulted or inferred while that contract is being
confirmed.

## Error Handling

- The tool includes retry logic for transient Azure API failures
- Use `--verbosity` flag to increase logging detail for troubleshooting
- Always use `--dry-run` first to preview changes before applying them

## HTTP golden (CI)

`TestHTTPGolden` uses `testutil.CompareWithFixture` against `internal/grafana/testdata/http/zz_fixture_TestHTTPGolden.json`. CI serves the recorded Grafana responses and asserts the current client still makes the same calls. No live Grafana in CI. A client change shows up as a golden diff — this PR's query change is `type=dash-db` without the old SDK's `starred=false`.

```bash
# rewrite the golden after an intentional client change
UPDATE=1 go test ./internal/grafana -run TestHTTPGolden -count=1
```

## Seeding the golden from live Grafana

`arohcp-dev` (resource group `global`) is how we seed the original response bodies. Read-only lists + one dashboard GET, with Grafana `limit=3` so the fixture stays small. Writes stay canned in the golden (we do not mutate the shared instance).

```bash
az account set --subscription "ARO Hosted Control Planes (EA Subscription 1)"
cd tools/grafanactl
GRAFANACTL_LIVE=1 GRAFANACTL_LIVE_UPDATE=1 go test ./internal/grafana -run TestSeedHTTPGolden -count=1 -v
```

To dump HTTP from any command (including INT):

```bash
GRAFANACTL_HTTP_DUMP_DIR=/tmp/grafana-http \
  go run . list datasources \
    --subscription "$SUBSCRIPTION" \
    --resource-group global \
    --grafana-name arohcp-dev
```
