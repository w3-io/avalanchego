#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat >&2 <<'EOF'
usage: run_bazel_impacted_tests.sh [options]

Options:
  --base-sha <sha>        Base commit to diff against. Defaults to BAZEL_IMPACTED_BASE_SHA.
  --scope <label-expr>    Bazel scope to search for impacted go_test rules. Repeatable.
  --fallback-task <task>  Task to run when no base SHA is available.
  --print-only            Print impacted test labels instead of running bazel test.
EOF
}

require_command() {
  local command_name="$1"
  command -v "$command_name" >/dev/null 2>&1 || {
    echo "ERROR: ${command_name} is required but not found on PATH" >&2
    exit 1
  }
}

base_sha="${BAZEL_IMPACTED_BASE_SHA:-}"
fallback_task=""
print_only=0
scopes=()

run_fallback_task() {
  if [[ -z "$fallback_task" ]]; then
    return 1
  fi

  echo "falling back to full task ${fallback_task}" >&2
  exec task "$fallback_task"
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --base-sha)
      base_sha="$2"
      shift 2
      ;;
    --scope)
      scopes+=("$2")
      shift 2
      ;;
    --fallback-task)
      fallback_task="$2"
      shift 2
      ;;
    --print-only)
      print_only=1
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

if [[ ${#scopes[@]} -eq 0 ]]; then
  echo "at least one --scope is required" >&2
  exit 2
fi

if [[ -z "$base_sha" ]]; then
  if [[ -n "$fallback_task" ]]; then
    echo "BAZEL_IMPACTED_BASE_SHA not set; running the full Bazel target set" >&2
    run_fallback_task
  fi

  echo "BAZEL_IMPACTED_BASE_SHA not set" >&2
  exit 2
fi

printf 'WARNING: BAZEL_IMPACTED_BASE_SHA is set; running only impacted tests against base %s\n' "$base_sha" >&2

require_command git
require_command bazelisk
if [[ -n "$fallback_task" ]]; then
  require_command task
fi

repo_root=$(git rev-parse --show-toplevel)
cd "$repo_root"

scratch_dir=$(mktemp -d)
impacted_targets="${scratch_dir}/impacted-targets.txt"
partition_tests="${scratch_dir}/partition-tests.txt"
impacted_tests="${scratch_dir}/impacted-tests.txt"

cleanup() {
  rm -rf "$scratch_dir"
}
trap cleanup EXIT

if ! ./scripts/print_bazel_impacted_targets.sh --base-sha "$base_sha" --output "$impacted_targets"; then
  echo "failed to compute impacted targets for base ${base_sha}" >&2
  run_fallback_task || exit $?
fi

partition_expr=""
for scope in "${scopes[@]}"; do
  if [[ -z "$partition_expr" ]]; then
    partition_expr="$scope"
  else
    partition_expr+=" union ${scope}"
  fi
done

partition_query="kind(\"go_test rule\", ${partition_expr}) except attr(\"tags\", \"manual\", kind(\"go_test rule\", ${partition_expr}))"
if ! bazelisk query "$partition_query" > "$partition_tests"; then
  echo "failed to query non-manual go_test targets for: ${partition_expr}" >&2
  run_fallback_task || exit $?
fi
grep -Fxf "$partition_tests" "$impacted_targets" > "$impacted_tests" || true

if (( print_only )); then
  cat "$impacted_tests"
  exit 0
fi

if [[ ! -s "$impacted_tests" ]]; then
  echo "no impacted test targets under: ${partition_expr}" >&2
  exit 0
fi

mapfile -t test_targets < "$impacted_tests"
printf 'running impacted tests:\n' >&2
printf '  %s\n' "${test_targets[@]}" >&2
exec bazelisk test "${test_targets[@]}"
