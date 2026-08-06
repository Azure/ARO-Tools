---
# Autonomous, go.work-aware Dependabot remediation for Azure/ARO-Tools.
# Reads open Dependabot alerts, groups them by package/cascade family, runs the
# `make tidy` workspace ritual, and opens one dependency-only PR per group.
#
# No PAT and no GitHub App: the built-in `dependabot` toolset reads alerts with
# the org-billed GITHUB_TOKEN once the workflow declares vulnerability-alerts:read
# + security-events:read. The Copilot agent is org-billed via copilot-requests.
# PR writes go through the safe-outputs job (see the note on the org policy below).

on:
  workflow_dispatch:            # manual run
  schedule: daily               # fuzzy daily sweep (scattered)

# Read-only for the agent. All write operations (opening PRs) are performed by
# the safe-outputs job with its own scoped, minimal permissions.
permissions:
  contents: read
  pull-requests: read
  security-events: read      # required by the dependabot toolset
  vulnerability-alerts: read # lets GITHUB_TOKEN read Dependabot alerts, no App
  copilot-requests: write    # org-billed engine inference via GITHUB_TOKEN, no PAT

engine: copilot

network: defaults

# Set up the Go toolchain on the runner using the version declared in the project
# (go.work), so the `make tidy` / `test-compile` ritual matches the workspace. No
# hardcoded version: go-version-file keeps this in lockstep with the repo.
steps:
  - name: Checkout
    uses: actions/checkout@v5
    with:
      persist-credentials: false
  - name: Set up Go from go.work
    uses: actions/setup-go@v5
    with:
      go-version-file: go.work

# The built-in GitHub MCP dependabot toolset reads the open alerts directly with
# the GITHUB_TOKEN (thanks to the vulnerability-alerts:read permission above). No
# App installation token, no app secrets, no per-install approval needed.
tools:
  github:
    toolsets: [dependabot, pull_requests]

# Writes are org-billed and scoped here, not in the frontmatter permissions above.
#
# PR creation uses Option B: the Azure org enforces "Allow GitHub Actions to create
# and approve pull requests" = OFF (org-level, the repo checkbox is greyed out and
# needs admin:org). That policy only governs the built-in GITHUB_TOKEN, so instead we
# mint an aro-hcp-robot GitHub App installation token for the safe-outputs job via
# `github-app:` below. App-authored PRs are not subject to the org policy, so no org
# change is needed. Requires two repo secrets for the aro-hcp-robot App (which has
# contents:write + pull_requests:write):
#   DEPENDABOT_APP_CLIENT_ID  = Iv23lioqETVAZNQE9L0P   (App CLIENT ID, not App ID 2853172)
#   DEPENDABOT_APP_PRIVATE_KEY = the App private-key PEM
# gh-aw passes client-id to actions/create-github-app-token (app-id is deprecated).
# No PAT.
safe-outputs:
  github-app:
    client-id: ${{ secrets.DEPENDABOT_APP_CLIENT_ID }}
    private-key: ${{ secrets.DEPENDABOT_APP_PRIVATE_KEY }}
  create-pull-request:
    max: 6                              # one PR per vulnerability group
    draft: true                         # open as draft, human marks ready
    title-prefix: "fix(deps): "
    labels: [dependencies, security, agentic-dependabot]

---

# Agentic Dependabot remediation for ARO-Tools (go.work-aware, PR-first)

You are remediating open Dependabot alerts for the `Azure/ARO-Tools` repository.
This is a Go multi-module `go.work` workspace. Read the module list from `go.work`
and the Go toolchain version from the `go`/`toolchain` directives in `go.work` (or the
modules' `go.mod`); do not assume a fixed version or module count, use whatever the
project declares. Native
Dependabot cannot handle it, because a per-manifest bump skips the workspace sync
and never re-tidies the other modules. Your job is to run that ritual correctly
and open one clean, dependency-only pull request per group.

## 1. Read the alerts

Use the GitHub `dependabot` toolset to list the open Dependabot alerts for this
repository (`Azure/ARO-Tools`). For each open alert capture:

- the ecosystem (package ecosystem, expect mostly `go`)
- the package name
- the vulnerable range and the first patched version
- severity and the GHSA/CVE identifier
- the module(s) where the dependency appears

Only consider alerts whose state is `open`. Ignore `dismissed` and `fixed` alerts.

## 1b. Skip vulnerabilities that already have an open PR

Before grouping, list the currently open pull requests in this repository (use the
`pull_requests` toolset). A vulnerability is already covered if an open PR bumps the
same package (match on the package name, the `fix(deps): ` title, or a referenced
GHSA/CVE in the PR title or body). Drop every alert that is already covered by an
open PR and do not reopen or duplicate it. Only carry forward alerts that have no
open PR. If, after this filter, no alerts remain, do nothing and open no PRs.

## 2. Group the alerts

Produce **one pull request per vulnerability group**. Group by remediation
family, not by individual alert:

- Group alerts for the **same package** together (all modules at once).
- Group a **cascade family** together: if bumping one module forces a coordinated
  bump across many workspace modules after `go work sync` (for example a
  `golang.org/x/*`, `k8s.io/*`, or `google.golang.org/grpc` bump that ripples
  through the workspace), that is a single group and a single PR.
- **Never mix ecosystems** in one PR. If any npm alerts show up, they are always
  separate PRs from Go fixes.

Aim for at most 6 groups. If there are more, prioritise by severity
(critical > high > medium > low).

## 3. Remediate each group

Work on a fresh branch per group, off the default branch. For each group:

1. Raise the dependency to the first patched version in the relevant module's
   `go.mod` (use `go get <module>@<version>` in each affected module directory).
2. Run the workspace ritual so the whole `go.work` stays consistent:
   `make tidy` (this runs `go mod tidy` in every module and then `go work sync`
   via the `work-sync` target). The tree must end tidy-clean.
3. Validate the change builds and lints:
   `make test-compile` and `make lint`. Both must pass. If `make lint-fix` is
   needed for trivial formatting, that is acceptable, but keep the diff
   dependency-only (see below).

## 4. Keep each PR dependency-only

Every PR must contain **only** dependency-management changes: `go.mod`, `go.sum`,
`go.work`, `go.work.sum`. No source edits, no unrelated churn. If the ritual pulls
in changes unrelated to the group, revert those files back to the default branch
before opening the PR.

## 5. Open the pull requests

For each group, open one **draft** pull request via the create-pull-request safe
output. The PR must:

- Title: `<package-or-family> to <version> (<severity>)`
  (the `fix(deps): ` prefix is added automatically, so give the rest).
- Body: list the alerts fixed (GHSA/CVE, package, from -> to version), the modules
  touched, and confirm the workspace is tidy-clean and `make test-compile` +
  `make lint` pass. State that it is dependency-only.
- Be dependency-only as described above.

Follow the repository conventions: plain, human wording, no em-dashes. Do not add
`Co-authored-by: Copilot` trailers. Do not create tracking issues. PRs only.

## 6. If you cannot fix a group

If a group has no patched version available, or the fix requires source changes
beyond dependency management (for example an API break from the bump), do not open
a broken PR. Report it via the missing-data / report-incomplete channel instead
and move on to the next group.
