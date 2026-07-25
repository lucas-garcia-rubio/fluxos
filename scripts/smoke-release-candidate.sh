#!/usr/bin/env bash

set -euo pipefail
umask 077

usage() {
  printf 'usage: %s <version> <artifact-dir>\n' "${0##*/}" >&2
  exit 2
}

die() {
  printf 'error: %s\n' "$*" >&2
  exit 1
}

warning() {
  printf 'warning: %s\n' "$*" >&2
}

require_command() {
  command -v "$1" >/dev/null 2>&1 || die "required command not found: $1"
}

[[ $# -eq 2 ]] || usage
version=$1
artifact_dir_arg=$2
[[ $version != -* ]] || die 'version must not start with -'
semver_re='^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-((0|[1-9][0-9]*|[0-9]*[A-Za-z-][0-9A-Za-z-]*)(\.(0|[1-9][0-9]*|[0-9]*[A-Za-z-][0-9A-Za-z-]*))*))?(\+([0-9A-Za-z-]+(\.[0-9A-Za-z-]+)*))?$'
[[ $version =~ $semver_re ]] || die "invalid SemVer version: $version"
[[ -n $artifact_dir_arg ]] || die 'artifact directory must not be empty'

export LC_ALL=C
export LANG=C
export TZ=UTC

require_command git
require_command readlink
require_command sha256sum
require_command cmp
require_command grep
require_command mktemp

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
  die 'could not resolve the script path'
fi
script_dir=${script_path%/*}
if ! script_root_raw=$(git -C "$script_dir" rev-parse --show-toplevel 2>/dev/null); then
  die 'the smoke script is not inside a Git worktree'
fi
if ! script_root=$(readlink -f -- "$script_root_raw"); then
  die 'could not resolve the smoke script worktree root'
fi
[[ $script_root == "$git_root" ]] || die 'the smoke script is not in the current Git worktree'
case $script_path in
  "$git_root"/*) ;;
  *) die 'the smoke script is outside the current Git worktree' ;;
esac

if ! tree_status=$(git -C "$git_root" status --porcelain=v1 --untracked-files=all); then
  die 'could not inspect the Git worktree'
fi
[[ -z $tree_status ]] || die 'Git worktree must be clean before the smoke'

artifact_candidate=$artifact_dir_arg
[[ $artifact_candidate == /* ]] || artifact_candidate="$working_dir/$artifact_candidate"
while [[ $artifact_candidate != / && $artifact_candidate == */ ]]; do
  artifact_candidate=${artifact_candidate%/}
done
[[ -d $artifact_candidate ]] || die 'artifact directory does not exist'
[[ ! -L $artifact_candidate ]] || die 'artifact directory must not be a symbolic link'
if ! artifact_dir=$(readlink -f -- "$artifact_candidate"); then
  die 'could not resolve the artifact directory'
fi
[[ $artifact_dir != "$git_root" && $artifact_dir != "$git_root/"* ]] || \
  die 'artifact directory must be outside the current Git worktree'

artifact_name="fluxos-$version-linux-amd64"
artifact="$artifact_dir/$artifact_name"
checksums="$artifact_dir/SHA256SUMS"
build_info="$artifact_dir/BUILD_INFO.txt"
for required_file in "$artifact" "$checksums" "$build_info"; do
  [[ -f $required_file && ! -L $required_file ]] || \
    die "promoted bundle is missing a regular file: $required_file"
done
[[ -x $artifact ]] || die 'promoted artifact is not executable'

if ! artifact_checksum_line=$(sha256sum -- "$artifact"); then
  die 'could not calculate the artifact checksum'
fi
artifact_checksum=${artifact_checksum_line%% *}

if ! temporary_root=$(mktemp -d /tmp/fluxos-release-smoke.XXXXXXXX); then
  die 'could not create the smoke temporary directory'
fi
cleanup() {
  local exit_code=$?
  rm -rf -- "$temporary_root" || true
  exit "$exit_code"
}
trap cleanup EXIT

expected_checksums="$temporary_root/expected-SHA256SUMS"
printf '%s  %s\n' "$artifact_checksum" "$artifact_name" > "$expected_checksums"
if ! cmp -s "$expected_checksums" "$checksums"; then
  die 'SHA256SUMS does not match the promoted artifact and stable base name'
fi
if ! (cd -- "$artifact_dir" && sha256sum -c -- SHA256SUMS); then
  die 'SHA256SUMS verification failed'
fi
printf 'bundle checksum: passed\n'

require_metadata_line() {
  local expected_line=$1
  grep -Fqx -- "$expected_line" "$build_info" || \
    die "BUILD_INFO.txt is missing: $expected_line"
}

require_metadata_line "artifact: $artifact_name"
require_metadata_line "version: $version"
require_metadata_line "sha256: $artifact_checksum"
require_metadata_line 'verification scope: same-host build repeatability only'
require_metadata_line 'rc gate: not evaluated'
grep -Eq '^commit: [0-9a-f]{40}$' "$build_info" || \
  die 'BUILD_INFO.txt has an invalid commit'
grep -Eq '^go: go version .+$' "$build_info" || \
  die 'BUILD_INFO.txt is missing Go toolchain metadata'
grep -Eq '^gcc target: [^[:space:]]+$' "$build_info" || \
  die 'BUILD_INFO.txt is missing GCC target metadata'
grep -Fq -- 'linker (' "$build_info" || die 'BUILD_INFO.txt is missing linker metadata'
grep -Fqx -- 'flags:' "$build_info" || die 'BUILD_INFO.txt is missing build flags'
grep -Eq '^host qualification: (ubuntu-24\.04-baseline|diagnostic)$' "$build_info" || \
  die 'BUILD_INFO.txt has an invalid host qualification'
grep -Eq '^host: GOHOSTOS=[^[:space:]]+ GOHOSTARCH=[^[:space:]]+ uname=[^/[:space:]]+/[^[:space:]]+$' "$build_info" || \
  die 'BUILD_INFO.txt has invalid host metadata'
grep -Eq '^os: ID=[^[:space:]]+ VERSION_ID=.*$' "$build_info" || \
  die 'BUILD_INFO.txt has invalid operating-system metadata'
if grep -Fq -- 'qualification: official' "$build_info"; then
  die 'BUILD_INFO.txt must not claim official qualification'
fi
printf 'bundle metadata: passed\n'

expected_version="$temporary_root/expected-version"
printf 'fluxos %s\n' "$version" > "$expected_version"

run_expected_output() {
  local label=$1
  local expected=$2
  shift 2
  local stdout_file="$temporary_root/$label.stdout"
  local stderr_file="$temporary_root/$label.stderr"

  if ! "$artifact" "$@" < /dev/null > "$stdout_file" 2> "$stderr_file"; then
    die "$label command failed"
  fi
  if ! cmp -s "$expected" "$stdout_file"; then
    die "$label stdout differs from its checked-in golden"
  fi
  [[ ! -s $stderr_file ]] || die "$label wrote to stderr"
  printf 'smoke: %s passed\n' "$label"
}

run_expected_output version "$expected_version" --version
run_expected_output m3-workflow-start "$git_root/testdata/trace/expected.mmd" \
  trace Workflow.start "$git_root/testdata/trace"
run_expected_output m4-no-prompt "$git_root/testdata/m4/interactive/expected-terminal.mmd" \
  trace --no-prompt app.Workflow.start "$git_root/testdata/m4/interactive"
run_expected_output m4-all-impls "$git_root/testdata/m4/interactive/expected-all-impls.mmd" \
  trace --all-impls app.Workflow.start "$git_root/testdata/m4/interactive"
run_expected_output m4-pick-alpha-gamma "$git_root/testdata/m4/interactive/expected-pick-alpha-gamma.mmd" \
  trace '--pick-impls=contracts.A=app.AlphaA,contracts.B=app.GammaB' \
  app.Workflow.start "$git_root/testdata/m4/interactive"
run_expected_output runtime-context-json "$git_root/testdata/m4/runtime-context/expected-start.json" \
  trace --format=json 'app.Workflow.start(app.First,app.Second)' "$git_root/testdata/m4/runtime-context"
run_expected_output runtime-context-dot "$git_root/testdata/m4/runtime-context/expected-start.dot" \
  trace --format=dot 'app.Workflow.start(app.First,app.Second)' "$git_root/testdata/m4/runtime-context"

petclinic_url='https://github.com/spring-projects/spring-petclinic.git'
petclinic_sha='c36452a2c34443ae26b4ecbba4f149906af14717'
petclinic_target='org.springframework.samples.petclinic.owner.OwnerController.processFindForm'
petclinic_dir="$temporary_root/spring-petclinic"
clone_stdout="$temporary_root/petclinic-clone.stdout"
clone_stderr="$temporary_root/petclinic-clone.stderr"
if ! git clone --no-checkout "$petclinic_url" "$petclinic_dir" > "$clone_stdout" 2> "$clone_stderr"; then
  die "Spring Petclinic clone failed; network or GitHub access is required"
fi
checkout_stdout="$temporary_root/petclinic-checkout.stdout"
checkout_stderr="$temporary_root/petclinic-checkout.stderr"
if ! git -C "$petclinic_dir" checkout --detach "$petclinic_sha" > "$checkout_stdout" 2> "$checkout_stderr"; then
  die "Spring Petclinic checkout failed for pinned commit $petclinic_sha"
fi
if ! actual_petclinic_sha=$(git -C "$petclinic_dir" rev-parse --verify HEAD); then
  die 'could not verify the Spring Petclinic checkout HEAD'
fi
[[ $actual_petclinic_sha == "$petclinic_sha" ]] || \
  die "Spring Petclinic HEAD mismatch: $actual_petclinic_sha"

printf 'spring petclinic: URL=%s SHA=%s target=%s\n' \
  "$petclinic_url" "$petclinic_sha" "$petclinic_target"
petclinic_stdout="$temporary_root/petclinic.stdout"
petclinic_stderr="$temporary_root/petclinic.stderr"
if ! "$artifact" trace "$petclinic_target" "$petclinic_dir" < /dev/null > "$petclinic_stdout" 2> "$petclinic_stderr"; then
  die 'Spring Petclinic smoke command failed'
fi
[[ ! -s $petclinic_stderr ]] || die 'Spring Petclinic smoke wrote to stderr'
petclinic_first_line=''
IFS= read -r petclinic_first_line < "$petclinic_stdout" || true
[[ $petclinic_first_line == 'flowchart TD' ]] || \
  die 'Spring Petclinic smoke did not render Mermaid flowchart TD'
grep -Fq -- "$petclinic_target" "$petclinic_stdout" || \
  die 'Spring Petclinic smoke output lacks a readable target reference'
printf 'smoke: spring-petclinic passed\n'

if ! final_tree_status=$(git -C "$git_root" status --porcelain=v1 --untracked-files=all); then
  die 'could not inspect the Git worktree after the smoke'
fi
[[ -z $final_tree_status ]] || die 'Git worktree is no longer clean after the smoke'
printf 'smoke result: passed\n'
