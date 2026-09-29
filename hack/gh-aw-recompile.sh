#!/usr/bin/env bash
set -euo pipefail

repo=${GITHUB_REPOSITORY:?GITHUB_REPOSITORY is required}
base=${GITHUB_BASE_REF:-main}
bot='aro-hcp-robot[bot]'
title='chore(gh-aw): [auto] upgrade ARO-Tools agentic workflows (AROSLSRE-2278)'

check_scope() {
  local file
  while IFS= read -r file; do
    [[ -z $file ]] && continue
    case "$file" in
      .github/workflows/dependabot-remediation.md|\
      .github/workflows/dependabot-remediation.lock.yml|\
      .github/aw/actions-lock.json|\
      .github/agents/agentic-workflows.agent.md|\
      .github/skills/agentic-workflows/SKILL.md|\
      .github/workflows/copilot-setup-steps.yml|\
      .gitattributes) ;;
      *) echo "Upgrade changed out-of-scope file: $file" >&2; exit 1 ;;
    esac
  done < <(git diff --name-only "origin/$base"...HEAD; git diff --cached --name-only; git ls-files --others --exclude-standard)
}

repair_agent() {
  local agent=.github/agents/agentic-workflows.agent.md
  local generated=.github/agents/agentic-workflows.md
  local version_output version commit
  if [[ -f $generated ]]; then
    mv "$generated" "$agent"
  fi
  [[ -f $agent ]] || { echo "gh-aw did not generate its dispatcher agent." >&2; exit 1; }
  version_output=$(gh aw version 2>&1) || { echo "gh aw version failed: $version_output" >&2; exit 1; }
  [[ $version_output =~ v[0-9]+\.[0-9]+\.[0-9]+ ]] ||
    { echo "Cannot determine installed gh-aw version." >&2; exit 1; }
  version=${BASH_REMATCH[0]}
  commit=$(gh api "repos/github/gh-aw/commits/$version" --jq '.sha')
  [[ $commit =~ ^[0-9a-f]{40}$ ]] || { echo "Cannot pin gh-aw prompts to a commit." >&2; exit 1; }
  sed -E -i "s@raw\.githubusercontent\.com/github/gh-aw/(main|refs/heads/main|[0-9a-f]{40})/@raw.githubusercontent.com/github/gh-aw/$commit/@g" "$agent"
  if grep -Eo 'raw\.githubusercontent\.com/github/gh-aw/(refs/heads/)?[^/]+/' "$agent" |
    grep -Fvx "raw.githubusercontent.com/github/gh-aw/$commit/"; then
    echo "An unpinned gh-aw prompt reference remains." >&2
    exit 1
  fi
}

check_changelog_exclusion() {
  grep -A2 '^    excluded-files:' .github/workflows/dependabot-remediation.md |
    grep -Fxq '      - CHANGELOG.md' ||
    { echo "Protected changelog exclusion missing from workflow source." >&2; exit 1; }
  local key count
  for key in GH_AW_SAFE_OUTPUTS_CONFIG GH_AW_SAFE_OUTPUTS_HANDLER_CONFIG; do
    count=$(grep -F "$key:" .github/workflows/dependabot-remediation.lock.yml |
      grep -Fc '\"excluded_files\":[\"CHANGELOG.md\"]' || true)
    [[ $count == 1 ]] ||
      { echo "Protected changelog exclusion missing from $key." >&2; exit 1; }
  done
}

if [[ ${1:-} == --repair-only ]]; then
  repair_agent
  exit 0
fi
if [[ ${1:-} == --check-scope-only ]]; then
  check_scope
  exit 0
fi
if [[ ${1:-} == --check-changelog-only ]]; then
  check_changelog_exclusion
  exit 0
fi

prs=$(gh pr list --repo "$repo" --state open --author app/aro-hcp-robot \
  --limit 500 --json number,headRefName,author)
matches=$(jq -c '[.[] | select(
  (.author.login == "app/aro-hcp-robot" or .author.login == "aro-hcp-robot[bot]") and
  (.headRefName | test("^upgrade-agentic-workflows-[0-9]+$"))
)]' <<<"$prs")
count=$(jq 'length' <<<"$matches")
(( count <= 1 )) || { echo "Multiple upgrade PRs match; refusing to choose." >&2; exit 1; }

if (( count == 1 )); then
  number=$(jq -r '.[0].number' <<<"$matches")
  branch=$(jq -r '.[0].headRefName' <<<"$matches")
  gh api "repos/$repo/pulls/$number" |
    jq -e --arg bot "$bot" --arg repo "$repo" --arg base "$base" --arg branch "$branch" \
      '.state == "open" and .user.login == $bot and .head.repo.full_name == $repo and .head.ref == $branch and .base.ref == $base' >/dev/null ||
    { echo "Upgrade PR identity, repository, or base does not match." >&2; exit 1; }
  if [[ ${1:-} == --select-only ]]; then
    printf '%s %s\n' "$number" "$branch"
    exit 0
  fi
  git fetch origin "refs/heads/$branch:refs/remotes/origin/$branch"
  git switch --detach "origin/$branch"
  git switch -c "$branch"
  git merge --no-edit "origin/$base"
  check_scope
else
  number=''
  branch="upgrade-agentic-workflows-${GITHUB_RUN_ID:?GITHUB_RUN_ID is required}"
  if [[ ${1:-} == --select-only ]]; then
    printf 'new %s\n' "$branch"
    exit 0
  fi
  git switch -c "$branch"
fi

gh aw upgrade --yes
repair_agent
gh aw compile --validate --no-emit
git add -A
check_scope
check_changelog_exclusion
git diff --cached --check
if git diff --cached --quiet; then
  echo "No upgrade changes."
  exit 0
fi
git commit -m "$title"
[[ -z $(git status --porcelain) ]] ||
  { echo "Verification left uncommitted changes; refusing to publish." >&2; exit 1; }

body=$(mktemp)
trap 'rm -f "$body"' EXIT
cat > "$body" <<'EOF'
[AROSLSRE-2278](https://redhat.atlassian.net/browse/AROSLSRE-2278)

### Problem

A blocked gh-aw compiler release prevents scheduled Dependabot remediation from activating.

### Goal

Keep the ARO-Tools agentic workflow compiled with a supported gh-aw release.

### What changes

Upgrade the compiler and its generated workflow artifacts in a scoped bot PR.

### Example

The weekly recompiler refreshes the generated workflow and action pins. An open upgrade PR is repaired in place.

### Validation

Compilation, changelog protection and the file scope are validated before pushing. CI and reviewer approval still gate merge.

### Follow-ups

Review the generated workflow permissions and protected-file exclusions.
EOF
git push origin "HEAD:refs/heads/$branch"
if [[ -n $number ]]; then
  gh pr edit "$number" --repo "$repo" --title "$title" --body-file "$body"
else
  gh pr create --repo "$repo" --base "$base" --head "$branch" \
    --title "$title" --body-file "$body"
fi
