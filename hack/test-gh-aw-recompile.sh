#!/usr/bin/env bash
set -euo pipefail

script=$(realpath "$(dirname "$0")/gh-aw-recompile.sh")
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
cat > "$tmp/gh" <<'EOF'
#!/usr/bin/env bash
case "$1 $2" in
  'pr list') printf '%s\n' "$MOCK_PR_LIST" ;;
  'api repos/Azure/ARO-Tools/pulls/42') printf '%s\n' "$MOCK_PR_DETAIL" ;;
  'aw version') printf '%s\n' "${MOCK_VERSION:-gh aw version v0.89.21}"; exit "${MOCK_VERSION_EXIT:-0}" ;;
  'api repos/github/gh-aw/commits/v0.89.21') echo 'c35393777e5604a63721d09512263b1383301d4f' ;;
  'pr create') echo 'https://github.com/Azure/ARO-Tools/pull/99' ;;
  'pr edit') : ;;
  *) echo "Unexpected gh invocation: $*" >&2; exit 1 ;;
esac
EOF
chmod +x "$tmp/gh"
export PATH="$tmp:$PATH" GITHUB_REPOSITORY=Azure/ARO-Tools GITHUB_RUN_ID=123
export MOCK_PR_LIST='[]'
[[ $(bash "$script" --select-only) == 'new upgrade-agentic-workflows-123' ]]

export MOCK_PR_LIST='[{"number":42,"headRefName":"upgrade-agentic-workflows-123","author":{"login":"app/aro-hcp-robot"}}]'
export MOCK_PR_DETAIL='{"state":"open","user":{"login":"aro-hcp-robot[bot]"},"head":{"repo":{"full_name":"Azure/ARO-Tools"},"ref":"upgrade-agentic-workflows-123"},"base":{"ref":"main"}}'
[[ $(bash "$script" --select-only) == '42 upgrade-agentic-workflows-123' ]]
MOCK_PR_LIST='[{"number":42,"headRefName":"upgrade-agentic-workflows-123","author":{"login":"other"}}]'
[[ $(bash "$script" --select-only) == 'new upgrade-agentic-workflows-123' ]]
MOCK_PR_LIST='[{"number":42,"headRefName":"upgrade-agentic-workflows-123","author":{"login":"app/aro-hcp-robot"}},{"number":43,"headRefName":"upgrade-agentic-workflows-124","author":{"login":"app/aro-hcp-robot"}}]'
if bash "$script" --select-only > /dev/null 2>&1; then
  echo "Ambiguous bot PRs must be rejected." >&2
  exit 1
fi
MOCK_PR_LIST='[{"number":42,"headRefName":"upgrade-agentic-workflows-123","author":{"login":"app/aro-hcp-robot"}}]'
MOCK_PR_DETAIL='{"state":"open","user":{"login":"aro-hcp-robot[bot]"},"head":{"repo":{"full_name":"someone/ARO-Tools"},"ref":"upgrade-agentic-workflows-123"},"base":{"ref":"main"}}'
if bash "$script" --select-only > /dev/null 2>&1; then
  echo "Fork PRs must be rejected." >&2
  exit 1
fi

export MOCK_PR_DETAIL='{"state":"open","user":{"login":"aro-hcp-robot[bot]"},"head":{"repo":{"full_name":"Azure/ARO-Tools"},"ref":"upgrade-agentic-workflows-123"},"base":{"ref":"main"}}'
git init -q --bare "$tmp/origin.git"
git init -q -b main "$tmp/repo"
git -C "$tmp/repo" config user.name Test
git -C "$tmp/repo" config user.email test@example.com
git -C "$tmp/repo" remote add origin "$tmp/origin.git"
echo base > "$tmp/repo/README.md"
git -C "$tmp/repo" add README.md
git -C "$tmp/repo" commit -qm base
git -C "$tmp/repo" push -q origin main
mkdir -p "$tmp/repo/.github/skills/agentic-workflows"
echo generated > "$tmp/repo/.github/skills/agentic-workflows/SKILL.md"
(cd "$tmp/repo" && bash "$script" --check-scope-only)
echo unrelated > "$tmp/repo/stray.txt"
if (cd "$tmp/repo" && bash "$script" --check-scope-only) > "$tmp/scope.log" 2>&1; then
  echo "Untracked out-of-scope files must be rejected." >&2
  exit 1
fi
rm "$tmp/repo/stray.txt" "$tmp/repo/.github/skills/agentic-workflows/SKILL.md"
git -C "$tmp/repo" switch -qc upgrade-agentic-workflows-123
echo unrelated > "$tmp/repo/unrelated.txt"
git -C "$tmp/repo" add unrelated.txt
git -C "$tmp/repo" commit -qm unrelated
git -C "$tmp/repo" push -q origin upgrade-agentic-workflows-123
git -C "$tmp/repo" switch -q main
git -C "$tmp/repo" branch -D upgrade-agentic-workflows-123 >/dev/null
git -C "$tmp/repo" switch -q --detach origin/upgrade-agentic-workflows-123
if (cd "$tmp/repo" && bash "$script" --check-scope-only) > "$tmp/scope.log" 2>&1; then
  echo "Committed out-of-scope files must be rejected." >&2
  exit 1
fi
git -C "$tmp/repo" switch -q main
if (cd "$tmp/repo" && bash "$script" --prepare-only) > "$tmp/scope.log" 2>&1; then
  echo "Out-of-scope bot PRs must be rejected." >&2
  exit 1
fi
grep -q 'out-of-scope file: unrelated.txt' "$tmp/scope.log"
git -C "$tmp/repo" switch -q main

mkdir -p "$tmp/guard/.github/workflows"
cat > "$tmp/guard/.github/workflows/dependabot-remediation.md" <<'EOF'
    excluded-files:
      - CHANGELOG.md
EOF
cat > "$tmp/guard/.github/workflows/dependabot-remediation.lock.yml" <<'EOF'
          GH_AW_SAFE_OUTPUTS_CONFIG: "{\"excluded_files\":[\"CHANGELOG.md\"]}"
          GH_AW_SAFE_OUTPUTS_HANDLER_CONFIG: "{\"excluded_files\":[\"CHANGELOG.md\"]}"
EOF
(cd "$tmp/guard" && bash "$script" --check-changelog-only)
for key in GH_AW_SAFE_OUTPUTS_CONFIG GH_AW_SAFE_OUTPUTS_HANDLER_CONFIG; do
  sed -i "/$key:/s/CHANGELOG.md/OTHER.md/" "$tmp/guard/.github/workflows/dependabot-remediation.lock.yml"
  if (cd "$tmp/guard" && bash "$script" --check-changelog-only) > "$tmp/changelog.log" 2>&1; then
    echo "Missing exclusion in $key must be rejected." >&2
    exit 1
  fi
  grep -q "Protected changelog exclusion missing from $key" "$tmp/changelog.log"
  sed -i "/$key:/s/OTHER.md/CHANGELOG.md/" "$tmp/guard/.github/workflows/dependabot-remediation.lock.yml"
done

cp -r "$tmp/guard/.github" "$tmp/repo/"
git -C "$tmp/repo" add .github
mkdir -p "$tmp/gh-aw-upgrade"
git -C "$tmp/repo" diff --cached --binary HEAD > "$tmp/gh-aw-upgrade/changes.patch"
jq -n --arg initial_sha "$(git -C "$tmp/repo" rev-parse HEAD)" \
  '{number:0,branch:"upgrade-agentic-workflows-999",initial_sha:$initial_sha,base_sha:$initial_sha}' \
  > "$tmp/gh-aw-upgrade/metadata.json"
git clone -q -b main "$tmp/origin.git" "$tmp/publish"
git -C "$tmp/publish" config user.name Test
git -C "$tmp/publish" config user.email test@example.com
(cd "$tmp/publish" && RUNNER_TEMP="$tmp" GITHUB_RUN_ID=999 bash "$script" --publish-only) > "$tmp/publish.log"
git -C "$tmp/publish" status --porcelain | grep -q . &&
  { echo "Publish must leave a clean worktree." >&2; exit 1; }
git -C "$tmp/publish" branch --show-current | grep -Fxq upgrade-agentic-workflows-999
git -C "$tmp/publish" diff --name-only HEAD^ HEAD |
  grep -Fxq .github/workflows/dependabot-remediation.lock.yml

git clone -q -b main "$tmp/origin.git" "$tmp/reuse"
git -C "$tmp/reuse" config user.name Test
git -C "$tmp/reuse" config user.email test@example.com
cp -r "$tmp/guard/.github" "$tmp/reuse/"
git -C "$tmp/reuse" add .github
git -C "$tmp/reuse" commit -qm 'Workflow baseline'
git -C "$tmp/reuse" push -q origin main
git -C "$tmp/reuse" switch -qc upgrade-agentic-workflows-456
mkdir -p "$tmp/reuse/.github/aw"
echo existing > "$tmp/reuse/.github/aw/actions-lock.json"
git -C "$tmp/reuse" add .github
git -C "$tmp/reuse" commit -qm 'Existing bot upgrade'
git -C "$tmp/reuse" push -q origin HEAD
initial_sha=$(git -C "$tmp/reuse" rev-parse HEAD)
git -C "$tmp/reuse" switch -q main
echo base-change >> "$tmp/reuse/README.md"
git -C "$tmp/reuse" add README.md
git -C "$tmp/reuse" commit -qm 'Unrelated base change'
git -C "$tmp/reuse" push -q origin main
base_sha=$(git -C "$tmp/reuse" rev-parse HEAD)
git -C "$tmp/reuse" switch -q upgrade-agentic-workflows-456
git -C "$tmp/reuse" merge -q --no-edit "$base_sha"
echo generated > "$tmp/reuse/.github/aw/actions-lock.json"
git -C "$tmp/reuse" add .github
git -C "$tmp/reuse" diff --cached --binary HEAD > "$tmp/gh-aw-upgrade/changes.patch"
if grep -q README.md "$tmp/gh-aw-upgrade/changes.patch"; then
  echo "Compiler patch must not contain base changes." >&2
  exit 1
fi
jq -n --arg initial_sha "$initial_sha" --arg base_sha "$base_sha" \
  '{number:42,branch:"upgrade-agentic-workflows-456",initial_sha:$initial_sha,base_sha:$base_sha}' \
  > "$tmp/gh-aw-upgrade/metadata.json"
export MOCK_PR_DETAIL
MOCK_PR_DETAIL=$(jq -n --arg sha "$initial_sha" \
  '{state:"open",user:{login:"aro-hcp-robot[bot]"},head:{repo:{full_name:"Azure/ARO-Tools"},ref:"upgrade-agentic-workflows-456",sha:$sha},base:{ref:"main"}}')
git clone -q -b main "$tmp/origin.git" "$tmp/publish-existing"
git -C "$tmp/publish-existing" config user.name Test
git -C "$tmp/publish-existing" config user.email test@example.com
(cd "$tmp/publish-existing" && RUNNER_TEMP="$tmp" bash "$script" --publish-only) > "$tmp/publish-existing.log"
git -C "$tmp/publish-existing" merge-base --is-ancestor "$base_sha" HEAD
[[ $(git -C "$tmp/publish-existing" show HEAD:README.md) == $'base\nbase-change' ]]
[[ $(git -C "$tmp/publish-existing" show HEAD:.github/aw/actions-lock.json) == generated ]]
if git -C "$tmp/publish-existing" diff --name-only "$base_sha"...HEAD | grep -Fxq README.md; then
  echo "Base changes must remain in merge ancestry, not the upgrade diff." >&2
  exit 1
fi

mkdir -p "$tmp/agent/.github/agents" "$tmp/agent/.github/skills/agentic-workflows"
git init -q "$tmp/agent"
echo 'https://raw.githubusercontent.com/github/gh-aw/main/example' > "$tmp/agent/.github/agents/agentic-workflows.md"
cat > "$tmp/agent/.github/skills/agentic-workflows/SKILL.md" <<'EOF'
Load these files from `github/gh-aw` (they are not available locally).
- `.github/aw/create-agentic-workflow.md`

When the task involves OTEL, OTLP, traces, observability backends, or telemetry-driven analysis, also read and follow `skills/otel-queries/SKILL.md` after loading the matching workflow prompt or skill.
EOF
(
  cd "$tmp/agent"
  if MOCK_VERSION='unparseable' bash "$script" --repair-only > "$tmp/version.log" 2>&1; then
    echo "Unparseable versions must be rejected." >&2
    exit 1
  fi
  grep -q 'Cannot determine installed gh-aw version' "$tmp/version.log"
  if MOCK_VERSION='unavailable' MOCK_VERSION_EXIT=1 bash "$script" --repair-only > "$tmp/version.log" 2>&1; then
    echo "Failed version command must be rejected." >&2
    exit 1
  fi
  grep -q 'gh aw version failed' "$tmp/version.log"
  bash "$script" --repair-only
  grep -q 'github/gh-aw/c35393777e5604a63721d09512263b1383301d4f/' \
    .github/agents/agentic-workflows.agent.md
  grep -Fq 'https://raw.githubusercontent.com/github/gh-aw/c35393777e5604a63721d09512263b1383301d4f/' \
    .github/skills/agentic-workflows/SKILL.md
  if grep -Fq 'skills/otel-queries/SKILL.md' .github/skills/agentic-workflows/SKILL.md; then
    echo "Generated dispatcher must not reference a missing OTEL skill." >&2
    exit 1
  fi
  grep -Fq -- '- `.github/aw/create-agentic-workflow.md`' .github/skills/agentic-workflows/SKILL.md
  git add .github/agents/agentic-workflows.agent.md .github/skills/agentic-workflows/SKILL.md
  git diff --cached --check
  bash "$script" --repair-only
  if grep -Fq 'skills/otel-queries/SKILL.md' .github/skills/agentic-workflows/SKILL.md; then
    echo "Repeat repair must not restore a missing OTEL route." >&2
    exit 1
  fi
  git add .github/skills/agentic-workflows/SKILL.md
  git diff --cached --check
  mkdir -p skills/otel-queries
  echo 'OTEL skill' > skills/otel-queries/SKILL.md
  echo 'When the task involves OTEL, OTLP, traces, observability backends, or telemetry-driven analysis, also read and follow `skills/otel-queries/SKILL.md` after loading the matching workflow prompt or skill.' >> .github/skills/agentic-workflows/SKILL.md
  bash "$script" --repair-only
  grep -Fq 'also read and follow `skills/otel-queries/SKILL.md`' .github/skills/agentic-workflows/SKILL.md
  rm -r skills/otel-queries
  echo 'Read `skills/otel-queries/SKILL.md` for telemetry.' >> .github/skills/agentic-workflows/SKILL.md
  if bash "$script" --repair-only > "$tmp/skill.log" 2>&1; then
    echo "Unknown missing-skill references must be rejected." >&2
    exit 1
  fi
  grep -q 'Generated dispatcher references a missing OTEL skill' "$tmp/skill.log"
  sed -i '/^Read `skills\/otel-queries\/SKILL.md` for telemetry\.$/d' .github/skills/agentic-workflows/SKILL.md
  echo 'Load https://raw.githubusercontent.com/github/gh-aw/main/unsafe.md' >> .github/skills/agentic-workflows/SKILL.md
  if bash "$script" --repair-only > "$tmp/skill.log" 2>&1; then
    echo "Unpinned skill references must be rejected." >&2
    exit 1
  fi
  grep -q 'An unpinned gh-aw prompt reference remains' "$tmp/skill.log"
)
echo "gh-aw recompile tests passed"
