#!/usr/bin/env bash

set -euo pipefail
umask 077

usage() {
  printf 'usage: %s [--diagnostic] <version> <output-dir>\n' "${0##*/}" >&2
  exit 2
}

die() {
  printf 'error: %s\n' "$*" >&2
  exit 1
}

require_command() {
  command -v "$1" >/dev/null 2>&1 || die "required command not found: $1"
}

diagnostic=0
if [[ $# -eq 3 && $1 == '--diagnostic' ]]; then
  diagnostic=1
  version=$2
  output_dir_arg=$3
elif [[ $# -eq 2 ]]; then
  version=$1
  output_dir_arg=$2
else
  usage
fi

[[ $version != -* ]] || die 'version must not start with -'
semver_re='^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-((0|[1-9][0-9]*|[0-9]*[A-Za-z-][0-9A-Za-z-]*)(\.(0|[1-9][0-9]*|[0-9]*[A-Za-z-][0-9A-Za-z-]*))*))?(\+([0-9A-Za-z-]+(\.[0-9A-Za-z-]+)*))?$'
[[ $version =~ $semver_re ]] || die "invalid SemVer version: $version"
[[ -n $output_dir_arg ]] || die 'output directory must not be empty'

export LC_ALL=C
export LANG=C
export TZ=UTC

require_command git
require_command readlink
require_command go
require_command find

if ! working_dir=$(pwd -P); then
  die 'could not determine the current directory'
fi
if ! git_root_raw=$(git rev-parse --show-toplevel 2>/dev/null); then
  die 'current directory is not inside a Git worktree'
fi
if ! git_root=$(readlink -f -- "$git_root_raw"); then
  die 'could not resolve the Git worktree root'
fi

if ! script_path=$(readlink -f -- "${BASH_SOURCE[0]}"); then
  die 'could not resolve the gate script path'
fi
script_dir=${script_path%/*}
if ! script_root_raw=$(git -C "$script_dir" rev-parse --show-toplevel 2>/dev/null); then
  die 'the gate script is not inside a Git worktree'
fi
if ! script_root=$(readlink -f -- "$script_root_raw"); then
  die 'could not resolve the gate script worktree root'
fi
[[ $script_root == "$git_root" ]] || die 'the gate script is not in the current Git worktree'
case $script_path in
  "$git_root"/*) ;;
  *) die 'the gate script is outside the current Git worktree' ;;
esac

if ! tree_status=$(git -C "$git_root" status --porcelain=v1 --untracked-files=all); then
  die 'could not inspect the Git worktree'
fi
[[ -z $tree_status ]] || die 'Git worktree must be clean before the release-candidate gate'

output_candidate=$output_dir_arg
[[ $output_candidate == /* ]] || output_candidate="$working_dir/$output_candidate"
while [[ $output_candidate != / && $output_candidate == */ ]]; do
  output_candidate=${output_candidate%/}
done
[[ -n $output_candidate ]] || die 'output directory path is empty'

if [[ -L $output_candidate ]]; then
  die 'output directory must not be a symbolic link'
elif [[ -e $output_candidate ]]; then
  [[ -d $output_candidate ]] || die 'output path must be a directory'
  if ! output_dir=$(readlink -f -- "$output_candidate"); then
    die 'could not resolve the output directory'
  fi
  if ! output_entries=$(find -- "$output_dir" -mindepth 1 -maxdepth 1 -print -quit); then
    die 'could not inspect the output directory'
  fi
  [[ -z $output_entries ]] || die 'output directory must be new or empty'
else
  output_parent=${output_candidate%/*}
  output_name=${output_candidate##*/}
  [[ -n $output_parent && -n $output_name ]] || die 'output directory parent must exist'
  [[ -d $output_parent ]] || die 'output directory parent must already exist'
  if ! output_parent=$(readlink -f -- "$output_parent"); then
    die 'could not resolve the output directory parent'
  fi
  output_dir="$output_parent/$output_name"
fi
[[ $output_dir != "$git_root" && $output_dir != "$git_root/"* ]] || \
  die 'output directory must be outside the current Git worktree'

run_local_check() {
  local label=$1
  shift
  printf 'local check: %s\n' "$label"
  if ! "$@"; then
    die "local check failed: $label"
  fi
}

if ! cd -- "$git_root"; then
  die 'could not enter the Git worktree root'
fi
run_local_check 'go test ./...' go test ./...
run_local_check 'go test -race ./...' go test -race ./...
run_local_check 'go vet ./...' go vet ./...

if ((diagnostic == 1)); then
  run_local_check 'reproducible build (diagnostic)' \
    "$git_root/scripts/verify-reproducible-build.sh" --diagnostic --output-dir "$output_dir" "$version"
else
  run_local_check 'reproducible build' \
    "$git_root/scripts/verify-reproducible-build.sh" --output-dir "$output_dir" "$version"
fi
run_local_check 'release-candidate smoke' \
  "$git_root/scripts/smoke-release-candidate.sh" "$version" "$output_dir"

if ! final_tree_status=$(git -C "$git_root" status --porcelain=v1 --untracked-files=all); then
  die 'could not inspect the Git worktree after the release-candidate checks'
fi
[[ -z $final_tree_status ]] || die 'Git worktree is no longer clean after the release-candidate checks'

printf 'local gate: passed\n'
printf 'remote CI: pending\n'
printf 'result: pending-remote-ci\n'
exit 3
