---
# Autonomous, go.work-aware Dependabot remediation for Azure/ARO-Tools.
# Reads open Dependabot alerts, groups them by package/cascade family, runs the
# `make tidy` workspace ritual, and opens one dependency-only PR per group.
#
# The Copilot engine runs under a secrecy/DIFC sandbox that filters private-scoped
# security data, so the agent itself cannot read Dependabot alerts through the GitHub
# MCP `dependabot` toolset (the response comes back empty and taints the agent's
# integrity label). We therefore fetch the alerts and the open PR list in ordinary
# Actions steps with an aro-hcp-robot App token (which has vulnerability_alerts:read)
# and hand the agent two JSON files in the workspace. The agent is org-billed via
# copilot-requests. A GitHub App is also used for the write side: PR creation goes
# through the safe-outputs job with an App installation token, never a PAT.

on:
  workflow_dispatch:            # manual run
  schedule: daily               # fuzzy daily sweep (scattered)

# The agent only needs to check out the repo and reach the org-billed engine. The
# alert/PR reads happen in the steps below with a scoped App token, and PR creation
# happens in the safe-outputs job with its own scoped App token.
permissions:
  contents: read
  copilot-requests: write    # org-billed engine inference via GITHUB_TOKEN, no PAT

engine: copilot

network: defaults

# Runner setup before the agent starts:
#  - check out the repo (persist-credentials:false is required by gh-aw strict mode),
#  - install the Go toolchain at the version declared in the project (go.work), so the
#    `make tidy` / `test-compile` ritual matches the workspace (no hardcoded version),
#  - mint a short-lived aro-hcp-robot App token and pre-fetch the open Dependabot alerts
#    and open PRs into workspace JSON files. These steps run on the runner, outside the
#    Copilot secrecy sandbox, so they can read the private-scoped alert data the agent
#    cannot. The files are excluded from git so they never leak into a remediation PR.
steps:
  - name: Checkout
    uses: actions/checkout@v5
    with:
      persist-credentials: false
  - name: Set up Go from go.work
    uses: actions/setup-go@v5
    with:
      go-version-file: go.work
  - name: Mint App token to read alerts and PRs
    id: read-token
    uses: actions/create-github-app-token@bcd2ba49218906704ab6c1aa796996da409d3eb1 # v3.2.0
    with:
      client-id: ${{ secrets.DEPENDABOT_APP_CLIENT_ID }}
      private-key: ${{ secrets.DEPENDABOT_APP_PRIVATE_KEY }}
      permission-contents: read
      permission-pull-requests: read
      permission-vulnerability-alerts: read
  - name: Pre-fetch open Dependabot alerts and open PRs
    env:
      GH_TOKEN: ${{ steps.read-token.outputs.token }}
    run: |
      set -euo pipefail
      # Keep the scratch files out of git so they never end up in a remediation PR.
      printf '%s\n' dependabot-alerts.json open-pull-requests.json >> .git/info/exclude
      gh api --paginate "/repos/${{ github.repository }}/dependabot/alerts?state=open&per_page=100" \
        --jq '.[] | {number, ecosystem: .dependency.package.ecosystem, package: .dependency.package.name, manifest: .dependency.manifest_path, ghsa: .security_advisory.ghsa_id, cve: .security_advisory.cve_id, severity: .security_advisory.severity, vulnerable_range: .security_vulnerability.vulnerable_version_range, first_patched: .security_vulnerability.first_patched_version.identifier}' \
        | jq -s '.' > dependabot-alerts.json
      gh api --paginate "/repos/${{ github.repository }}/pulls?state=open&per_page=100" \
        --jq '.[] | {number, title, head: .head.ref, draft: .draft}' \
        | jq -s '.' > open-pull-requests.json
      echo "Fetched $(jq length dependabot-alerts.json) open alerts and $(jq length open-pull-requests.json) open PRs"

# PR creation is scoped here, not by the frontmatter permissions above. These writes are
# NOT performed with the org-billed GITHUB_TOKEN; they use a GitHub App installation token
# minted for the safe-outputs job (see below).
#
# PR creation uses Option B: the Azure org enforces "Allow GitHub Actions to create
# and approve pull requests" = OFF (org-level, the repo checkbox is greyed out and
# needs admin:org). That policy only governs the built-in GITHUB_TOKEN, so instead we
# mint an aro-hcp-robot GitHub App installation token for the safe-outputs job via
# `github-app:` below. App-authored PRs are not subject to the org policy, so no org
# change is needed. Requires two repo secrets for the aro-hcp-robot App (which has
# contents:write + pull_requests:write):
#   DEPENDABOT_APP_CLIENT_ID   = the App's OAuth client ID (not the numeric App ID)
#   DEPENDABOT_APP_PRIVATE_KEY = the App private-key PEM
# fallback-as-issue:false keeps the minted token down to contents:write + pull_requests:write
# (no issues:write), matching what the App installation grants. No PAT.
safe-outputs:
  github-app:
    client-id: ${{ secrets.DEPENDABOT_APP_CLIENT_ID }}
    private-key: ${{ secrets.DEPENDABOT_APP_PRIVATE_KEY }}
  create-pull-request:
    max: 6                              # one PR per vulnerability group
    draft: true                         # open as draft, human marks ready
    fallback-as-issue: false            # no issues: write on the App token, fail instead of opening an issue
    title-prefix: "fix(deps): "
    labels: [dependencies, security, agentic-dependabot]

---

# Agentic Dependabot remediation for ARO-Tools

You are remediating open Dependabot alerts for the `Azure/ARO-Tools` repository. This is a Go multi-module `go.work` workspace. Read the module list from `go.work` and the Go toolchain version from the `go`/`toolchain` directives in `go.work` (or the modules' `go.mod`); do not assume a fixed version or module count, use whatever the project declares. Native Dependabot cannot handle it, because a per-manifest bump skips the workspace sync and never re-tidies the other modules. Your job is to run that ritual correctly and open one clean, dependency-only pull request per group.

## 1. Read the alerts

The open Dependabot alerts have already been fetched for you into `dependabot-alerts.json` in the repository root (the Copilot secrecy sandbox blocks the agent from reading the Dependabot API directly, so a prior workflow step fetched them with an App token). Read that file. It is a JSON array; each entry has:

- `ecosystem` (package ecosystem, expect mostly `go`)
- `package` (the module path)
- `vulnerable_range` and `first_patched` (the first patched version)
- `severity` and the `ghsa` / `cve` identifiers
- `manifest` (the manifest path where the dependency appears)
- `number` (the alert number)

Every entry in the file is already an `open` alert. If the file is empty (`[]`), there is nothing to do: open no PRs and finish.

## 1b. Skip vulnerabilities that already have an open PR

The currently open pull requests have been fetched into `open-pull-requests.json` in the repository root (a JSON array of `{number, title, head, draft}`). Read that file. A vulnerability is already covered if an open PR bumps the same package (match on the package name, the `fix(deps): ` title, or a referenced GHSA/CVE in the PR title). Drop every alert that is already covered by an open PR and do not reopen or duplicate it. Only carry forward alerts that have no open PR. If, after this filter, no alerts remain, do nothing and open no PRs.

## 2. Group the alerts

Produce **one pull request per vulnerability group**. Group by remediation family, not by individual alert:

- Group alerts for the **same package** together (all modules at once).
- Group a **cascade family** together: if bumping one module forces a coordinated bump across many workspace modules after `go work sync` (for example a `golang.org/x/*`, `k8s.io/*`, or `google.golang.org/grpc` bump that ripples through the workspace), that is a single group and a single PR.
- **Never mix ecosystems** in one PR. If any npm alerts show up, they are always separate PRs from Go fixes.

Aim for at most 6 groups. If there are more, prioritise by severity (critical > high > medium > low).

## 3. Remediate each group

Work on a fresh branch per group, off the default branch. For each group:

1. Raise the dependency to the first patched version in the relevant module's `go.mod` (use `go get <module>@<version>` in each affected module directory).
2. Run the workspace ritual so the whole `go.work` stays consistent: `make tidy` (this runs `go mod tidy` in every module and then `go work sync` via the `work-sync` target). The tree must end tidy-clean.
3. Validate the change builds and lints: `make test-compile` and `make lint`. Both must pass. If `make lint-fix` is needed for trivial formatting, that is acceptable, but keep the diff dependency-only (see below).

## 4. Keep each PR dependency-only

Every PR must contain **only** dependency-management changes: `go.mod`, `go.sum`, `go.work`, `go.work.sum`. No source edits, no unrelated churn. Never include the `dependabot-alerts.json` or `open-pull-requests.json` scratch files. If the ritual pulls in changes unrelated to the group, revert those files back to the default branch before opening the PR.

## 5. Open the pull requests

For each group, open one **draft** pull request via the create-pull-request safe output. The PR must:

- Title: `<package-or-family> to <version> (<severity>)` (the `fix(deps): ` prefix is added automatically, so give the rest).
- Body: list the alerts fixed (GHSA/CVE, package, from -> to version), the modules touched, and confirm the workspace is tidy-clean and `make test-compile` + `make lint` pass. State that it is dependency-only.
- Be dependency-only as described above.

Follow the repository conventions: plain, human wording, no em-dashes. Do not add `Co-authored-by: Copilot` trailers. Do not create tracking issues. PRs only.

## 6. If you cannot fix a group

If a group has no patched version available, or the fix requires source changes beyond dependency management (for example an API break from the bump), do not open a broken PR. Report it via the missing-data / report-incomplete channel instead and move on to the next group.
