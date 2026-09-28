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
if (cd "$tmp/repo" && bash "$script") > "$tmp/scope.log" 2>&1; then
  echo "Out-of-scope bot PRs must be rejected." >&2
  exit 1
fi
grep -q 'out-of-scope file: unrelated.txt' "$tmp/scope.log"

mkdir -p "$tmp/agent/.github/agents"
echo 'https://raw.githubusercontent.com/github/gh-aw/main/example' > "$tmp/agent/.github/agents/agentic-workflows.md"
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
)
echo "gh-aw recompile tests passed"
