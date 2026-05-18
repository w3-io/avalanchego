#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat >&2 <<'EOF'
usage: print_bazel_impacted_targets.sh [options]

Print newline-delimited impacted Bazel targets to stdout.

By default, this compares the current working tree (including uncommitted
changes) against HEAD. Pass --base-sha to compare against a different commit.

Options:
  --base-sha <sha>   Base commit to diff against. Defaults to BAZEL_IMPACTED_BASE_SHA or HEAD.
  --output <path>    Write impacted targets to this file instead of stdout.
  --verbose          Print progress messages to stderr.
EOF
}

require_command() {
  local command_name="$1"
  command -v "$command_name" >/dev/null 2>&1 || {
    echo "ERROR: ${command_name} is required but not found on PATH" >&2
    exit 1
  }
}

log() {
  if (( verbose )); then
    printf '%s\n' "$*" >&2
  fi
}

base_sha="${BAZEL_IMPACTED_BASE_SHA:-}"
output_path=""
verbose=0

while [[ $# -gt 0 ]]; do
  case "$1" in
    --base-sha)
      base_sha="$2"
      shift 2
      ;;
    --output)
      output_path="$2"
      shift 2
      ;;
    --verbose)
      verbose=1
      shift
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "unknown argument: $1" >&2
      usage
      exit 2
      ;;
  esac
done

require_command git
require_command jq
require_command nix
require_command bazelisk

repo_root=$(git rev-parse --show-toplevel)
cd "$repo_root"

if [[ -z "$base_sha" ]]; then
  base_sha=$(git rev-parse HEAD)
fi

if ! git cat-file -e "${base_sha}^{commit}" 2>/dev/null; then
  log "fetching base commit ${base_sha}"
  git fetch --no-tags --depth=1 origin "$base_sha"
fi

scratch_dir=$(mktemp -d)
base_worktree="${scratch_dir}/base"
base_hashes="${scratch_dir}/base-hashes.json"
head_hashes="${scratch_dir}/head-hashes.json"
impacted_targets="${scratch_dir}/impacted-targets.txt"
bazel_diff_url="https://github.com/Tinder/bazel-diff/releases/download/v22.0.0/bazel-diff_deploy.jar"
bazel_diff_hash="sha256-F7opo1MmvosIVObhv29FA2gq9bBBZfeaPn+96apq5uY="

cleanup() {
  git worktree remove --force "$base_worktree" >/dev/null 2>&1 || true
  rm -rf "$scratch_dir"
}
trap cleanup EXIT

log "creating base worktree at ${base_sha}"
git worktree add --detach "$base_worktree" "$base_sha" >/dev/null

prefetch_json=$(nix store prefetch-file --json "$bazel_diff_url")
actual_hash=$(jq -r '.hash' <<<"$prefetch_json")
if [[ "$actual_hash" != "$bazel_diff_hash" ]]; then
  echo "unexpected bazel-diff hash: ${actual_hash}" >&2
  exit 1
fi
bazel_diff_jar=$(jq -r '.storePath' <<<"$prefetch_json")
bazel_path=$(command -v bazelisk)
java_path=$(command -v java || true)
run_bazel_diff() {
  if [[ -n "$java_path" ]]; then
    "$java_path" -jar "$bazel_diff_jar" "$@"
    return
  fi

  nix run nixpkgs#jdk_headless -- -jar "$bazel_diff_jar" "$@"
}

log "generating base hashes"
run_bazel_diff generate-hashes \
  -w "$base_worktree" \
  -b "$bazel_path" \
  "$base_hashes"

log "generating working-tree hashes"
run_bazel_diff generate-hashes \
  -w "$repo_root" \
  -b "$bazel_path" \
  "$head_hashes"

log "computing impacted targets"
run_bazel_diff get-impacted-targets \
  -w "$repo_root" \
  -b "$bazel_path" \
  -sh "$base_hashes" \
  -fh "$head_hashes" \
  -o "$impacted_targets"

if [[ -n "$output_path" ]]; then
  cp "$impacted_targets" "$output_path"
  log "wrote impacted targets to ${output_path}"
else
  cat "$impacted_targets"
fi
