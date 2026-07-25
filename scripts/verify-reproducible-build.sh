#!/usr/bin/env bash

set -euo pipefail
umask 077

usage() {
  printf 'usage: %s [--diagnostic] [--output-dir DIR] <version>\n' "${0##*/}" >&2
  exit 2
}

die() {
  printf 'error: %s\n' "$*" >&2
  exit 1
}

warning() {
  printf 'warning: %s\n' "$*" >&2
}

diagnostic=0
diagnostic_seen=0
output_dir_arg=''
output_dir_seen=0
version=''

while (($# > 0)); do
  case $1 in
    --diagnostic)
      ((diagnostic_seen == 0)) || usage
      diagnostic=1
      diagnostic_seen=1
      shift
      ;;
    --output-dir)
      ((output_dir_seen == 0)) || usage
      (($# >= 2)) || usage
      [[ -n $2 ]] || die '--output-dir requires a non-empty directory'
      output_dir_arg=$2
      output_dir_seen=1
      shift 2
      ;;
    *)
      version=$1
      shift
      break
      ;;
  esac
done

[[ -n $version && $# -eq 0 ]] || usage

[[ $version != -* ]] || die 'version must not start with -'

# SemVer 2.0.0, with the required v prefix.
semver_re='^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-((0|[1-9][0-9]*|[0-9]*[A-Za-z-][0-9A-Za-z-]*)(\.(0|[1-9][0-9]*|[0-9]*[A-Za-z-][0-9A-Za-z-]*))*))?(\+([0-9A-Za-z-]+(\.[0-9A-Za-z-]+)*))?$'
[[ $version =~ $semver_re ]] || die "invalid SemVer version: $version"

export LC_ALL=C
export LANG=C
export TZ=UTC

require_command() {
  command -v "$1" >/dev/null 2>&1 || die "required command not found: $1"
}

require_command git
require_command readlink

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
  die 'the verification script is not inside a Git worktree'
fi
if ! script_root=$(readlink -f -- "$script_root_raw"); then
  die 'could not resolve the script worktree root'
fi
[[ $script_root == "$git_root" ]] || die 'the verification script is not in the current Git worktree'
case $script_path in
  "$git_root"/*) ;;
  *) die 'the verification script is outside the current Git worktree' ;;
esac

output_dir=''
output_dir_new=0
if ((output_dir_seen == 1)); then
  require_command find
  require_command stat

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
    output_parent=${output_dir%/*}
    [[ -n $output_parent ]] || output_parent=/
  else
    output_parent=${output_candidate%/*}
    output_name=${output_candidate##*/}
    [[ -n $output_parent && -n $output_name ]] || die 'output directory parent must exist'
    [[ -d $output_parent ]] || die 'output directory parent must already exist'
    if ! output_parent=$(readlink -f -- "$output_parent"); then
      die 'could not resolve the output directory parent'
    fi
    output_dir="$output_parent/$output_name"
    output_dir_new=1
  fi

  [[ $output_dir != "$git_root" && $output_dir != "$git_root/"* ]] || \
    die 'output directory must be outside the current Git worktree'

  if ((output_dir_new == 0)); then
    if ! output_entries=$(find -- "$output_dir" -mindepth 1 -maxdepth 1 -print -quit); then
      die 'could not inspect the output directory'
    fi
    [[ -z $output_entries ]] || die 'output directory must be new or empty'
  fi
fi

if ! initial_status=$(git -C "$git_root" status --porcelain=v1 --untracked-files=all); then
  die 'could not inspect the Git worktree'
fi
[[ -z $initial_status ]] || die 'Git worktree must be clean, including non-ignored untracked files'
if ! initial_head=$(git -C "$git_root" rev-parse --verify HEAD); then
  die 'could not resolve HEAD'
fi

unset GOFLAGS GOEXPERIMENT SOURCE_DATE_EPOCH \
  CGO_CFLAGS CGO_CPPFLAGS CGO_CXXFLAGS CGO_FFLAGS CGO_LDFLAGS \
  CGO_CFLAGS_ALLOW CGO_CFLAGS_DISALLOW CGO_CXXFLAGS_ALLOW CGO_CXXFLAGS_DISALLOW \
  CGO_FFLAGS_ALLOW CGO_FFLAGS_DISALLOW CGO_LDFLAGS_ALLOW CGO_LDFLAGS_DISALLOW
export GOENV=off
export GOTOOLCHAIN=local
export CGO_ENABLED=1
export CC=gcc
export GOOS=linux
export GOARCH=amd64
export GOAMD64=v1

require_command go
require_command gcc
require_command uname
require_command mktemp
require_command cmp
require_command sha256sum

if ! go_host_os=$(go env GOHOSTOS); then
  die 'could not determine GOHOSTOS'
fi
if ! go_host_arch=$(go env GOHOSTARCH); then
  die 'could not determine GOHOSTARCH'
fi
if ! uname_s=$(uname -s); then
  die 'could not determine the host kernel'
fi
if ! uname_m=$(uname -m); then
  die 'could not determine the host machine'
fi
if ! gcc_target=$(gcc -dumpmachine); then
  die 'could not determine the GCC target'
fi

os_id=''
os_version_id=''
if [[ -r /etc/os-release ]]; then
  while IFS= read -r os_release_line || [[ -n $os_release_line ]]; do
    case $os_release_line in
      ID=*) os_id=${os_release_line#ID=} ;;
      VERSION_ID=*) os_version_id=${os_release_line#VERSION_ID=} ;;
    esac
  done < /etc/os-release
fi

strip_optional_quotes() {
  local value=$1
  case $value in
    "'"*"'") value=${value:1:${#value}-2} ;;
    '"'*'"') value=${value:1:${#value}-2} ;;
  esac
  printf '%s' "$value"
}

os_id=$(strip_optional_quotes "$os_id")
os_version_id=$(strip_optional_quotes "$os_version_id")

host_issues=()
[[ $go_host_os == linux ]] || host_issues+=("GOHOSTOS=$go_host_os")
[[ $go_host_arch == amd64 ]] || host_issues+=("GOHOSTARCH=$go_host_arch")
[[ $uname_s == Linux ]] || host_issues+=("uname -s=$uname_s")
[[ $uname_m == x86_64 ]] || host_issues+=("uname -m=$uname_m")
[[ $gcc_target =~ ^x86_64[-_][^[:space:]]*linux[^[:space:]]*$ ]] || \
  die "gcc target is not capable of x86_64 Linux: $gcc_target"
[[ $os_id == ubuntu && $os_version_id == 24.04 ]] || \
  host_issues+=("OS=${os_id:-unknown} ${os_version_id:-unknown}, expected Ubuntu 24.04")

if ((${#host_issues[@]} > 0)); then
  if ((diagnostic == 0)); then
    die "host does not match the Ubuntu 24.04 native linux/amd64 baseline (${host_issues[*]}); rerun with --diagnostic for a non-qualifying run"
  fi
  warning "diagnostic evidence only on a non-baseline host (${host_issues[*]}); it is not release qualification"
elif ((diagnostic == 1)); then
  warning '--diagnostic was requested; this run is diagnostic evidence only and not release qualification'
fi

if ((diagnostic == 1)); then
  host_qualification=diagnostic
else
  host_qualification=ubuntu-24.04-baseline
fi

if ! go_version=$(go version); then
  die 'could not determine the Go version'
fi
if ! gcc_version=$(gcc --version); then
  die 'could not determine the GCC version'
fi
if ! linker=$(gcc -print-prog-name=ld); then
  die 'could not determine the linker used by GCC'
fi
[[ -n $linker ]] || die 'GCC returned an empty linker name'
if ! linker_version=$("$linker" --version); then
  die "could not determine the linker version: $linker"
fi

printf 'reproducible build verification\n'
printf 'version: %s\n' "$version"
printf 'commit: %s\n' "$initial_head"
printf 'host: GOHOSTOS=%s GOHOSTARCH=%s uname=%s/%s\n' \
  "$go_host_os" "$go_host_arch" "$uname_s" "$uname_m"
printf 'os: ID=%s VERSION_ID=%s\n' "${os_id:-unknown}" "${os_version_id:-unknown}"
printf 'host qualification: %s\n' "$host_qualification"
printf 'verification scope: same-host build repeatability only\n'
printf 'rc gate: not evaluated\n'
printf 'gcc target: %s\n' "$gcc_target"
printf 'go: %s\n' "$go_version"
printf 'gcc:\n%s\n' "$gcc_version"
printf 'linker (%s):\n%s\n' "$linker" "$linker_version"
printf 'environment:\n'
for environment_name in \
  GOENV GOTOOLCHAIN CGO_ENABLED CC GOOS GOARCH GOAMD64 LC_ALL LANG TZ \
  GOFLAGS GOEXPERIMENT SOURCE_DATE_EPOCH \
  CGO_CFLAGS CGO_CPPFLAGS CGO_CXXFLAGS CGO_FFLAGS CGO_LDFLAGS; do
  if [[ -v $environment_name ]]; then
    printf '  %s=%s\n' "$environment_name" "${!environment_name}"
  else
    printf '  %s=<unset>\n' "$environment_name"
  fi
done

if ! cd -- "$git_root"; then
  die 'could not enter the Git worktree root'
fi
if ! go mod verify; then
  die 'go mod verify failed'
fi

if ! temporary_root=$(mktemp -d /tmp/fluxos-reproducible-build.XXXXXXXX); then
  die 'could not create a secure temporary directory under /tmp'
fi

staging_dir=''
bundle_published=0
published_identity=''
artifact_path=''
checksums_path=''
build_info_path=''
cleanup() {
  local exit_status=$?

  if ((exit_status != 0 && bundle_published == 1)); then
    local current_identity=''
    if current_identity=$(stat -c '%d:%i' -- "$output_dir" 2>/dev/null) && \
      [[ -n $published_identity && $current_identity == "$published_identity" ]]; then
      rm -f -- "$artifact_path" "$checksums_path" "$build_info_path" || true
      rmdir -- "$output_dir" 2>/dev/null || true
    fi
  fi
  if [[ -n $staging_dir && ( -e $staging_dir || -L $staging_dir ) ]]; then
    rm -rf -- "$staging_dir" || true
  fi
  rm -rf -- "$temporary_root" || true
  exit "$exit_status"
}
trap cleanup EXIT

if ! cache_one=$(mktemp -d "$temporary_root/cache-one.XXXXXXXX"); then
  die 'could not create the first temporary GOCACHE'
fi
if ! cache_two=$(mktemp -d "$temporary_root/cache-two.XXXXXXXX"); then
  die 'could not create the second temporary GOCACHE'
fi
temporary_output_dir="$temporary_root/bin"
if ! mkdir -- "$temporary_output_dir"; then
  die 'could not create the temporary binary directory'
fi

binary_one="$temporary_output_dir/fluxos-build-one"
binary_two="$temporary_output_dir/fluxos-build-two"

build_binary() {
  local cache=$1
  local output=$2
  export GOCACHE=$cache
  go build -a -trimpath -buildvcs=false -mod=readonly \
    -ldflags "-s -w -X main.version=$version" \
    -o "$output" ./cmd/fluxos
}

printf 'build 1: GOCACHE=%s\n' "$cache_one"
if ! build_binary "$cache_one" "$binary_one"; then
  die 'first reproducible build failed'
fi
printf 'build 2: GOCACHE=%s\n' "$cache_two"
if ! build_binary "$cache_two" "$binary_two"; then
  die 'second reproducible build failed'
fi

build_id_one=$(go tool buildid "$binary_one")
build_id_two=$(go tool buildid "$binary_two")
printf 'build ID (build 1): %s\n' "$build_id_one"
printf 'build ID (build 2): %s\n' "$build_id_two"

expected_version="$temporary_root/expected-version"
printf 'fluxos %s\n' "$version" > "$expected_version"

verify_version_output() {
  local binary=$1
  local label=$2
  local stdout_file="$temporary_root/$label.stdout"
  local stderr_file="$temporary_root/$label.stderr"

  if ! "$binary" --version >"$stdout_file" 2>"$stderr_file"; then
    die "$label --version failed"
  fi
  if ! cmp -s "$expected_version" "$stdout_file"; then
    die "$label --version did not print exactly: fluxos $version"
  fi
  [[ ! -s $stderr_file ]] || die "$label --version wrote to stderr"
}

verify_version_output "$binary_one" build-one
verify_version_output "$binary_two" build-two

sha256_of() {
  local checksum_line
  checksum_line=$(sha256sum -- "$1")
  printf '%s' "${checksum_line%% *}"
}

verify_worktree_state() {
  local current_head current_status

  if ! current_head=$(git -C "$git_root" rev-parse --verify HEAD); then
    die 'could not resolve HEAD after the build'
  fi
  [[ $current_head == "$initial_head" ]] || die 'HEAD changed during verification'
  if ! current_status=$(git -C "$git_root" status --porcelain=v1 --untracked-files=all); then
    die 'could not inspect the Git worktree after the build'
  fi
  [[ -z $current_status ]] || die 'Git worktree is no longer clean after the build'
}

promote_output() {
  local staged_artifact staged_checksums staged_build_info staged_compare_one staged_compare_two
  local artifact_base artifact_sha current_identity

  ((output_dir_seen == 1)) || return 0

  artifact_base="fluxos-$version-linux-amd64"
  if ! staging_dir=$(mktemp -d "$output_parent/.fluxos-release-staging.XXXXXXXX"); then
    die 'could not create the temporary promotion directory'
  fi
  staged_artifact="$staging_dir/$artifact_base"
  staged_checksums="$staging_dir/SHA256SUMS"
  staged_build_info="$staging_dir/BUILD_INFO.txt"
  staged_compare_one="$staging_dir/.fluxos-build-one"
  staged_compare_two="$staging_dir/.fluxos-build-two"

  if ! cp -- "$binary_one" "$staged_compare_one" || \
    ! cp -- "$binary_two" "$staged_compare_two"; then
    die 'could not stage the reproducible builds'
  fi
  if ! cmp -s "$staged_compare_one" "$staged_compare_two"; then
    die 'staged reproducible builds differ'
  fi
  if ! cp -- "$staged_compare_one" "$staged_artifact"; then
    die 'could not stage the reproducible binary'
  fi
  if ! chmod 0755 -- "$staged_artifact"; then
    die 'could not make the staged binary executable'
  fi
  artifact_sha=$(sha256_of "$staged_artifact")
  printf '%s  %s\n' "$artifact_sha" "$artifact_base" > "$staged_checksums"
  {
    printf 'artifact: %s\n' "$artifact_base"
    printf 'commit: %s\n' "$initial_head"
    printf 'version: %s\n' "$version"
    printf 'host qualification: %s\n' "$host_qualification"
    printf 'verification scope: same-host build repeatability only\n'
    printf 'rc gate: not evaluated\n'
    printf 'go: %s\n' "$go_version"
    printf 'gcc target: %s\n' "$gcc_target"
    printf 'gcc:\n%s\n' "$gcc_version"
    printf 'linker (%s):\n%s\n' "$linker" "$linker_version"
    printf 'host: GOHOSTOS=%s GOHOSTARCH=%s uname=%s/%s\n' \
      "$go_host_os" "$go_host_arch" "$uname_s" "$uname_m"
    printf 'os: ID=%s VERSION_ID=%s\n' "${os_id:-unknown}" "${os_version_id:-unknown}"
    printf 'sha256: %s\n' "$artifact_sha"
    printf 'flags:\n'
    printf '  go build: -a -trimpath -buildvcs=false -mod=readonly\n'
    printf '  ldflags: -s -w -X main.version=%s\n' "$version"
    printf '  environment: GOENV=off GOTOOLCHAIN=local CGO_ENABLED=1 CC=gcc GOOS=linux GOARCH=amd64 GOAMD64=v1 LC_ALL=C LANG=C TZ=UTC\n'
    printf '  cleared: GOFLAGS GOEXPERIMENT SOURCE_DATE_EPOCH CGO_*FLAGS\n'
    printf '  GOCACHE: two independent temporary directories\n'
  } > "$staged_build_info"

  verify_version_output "$staged_artifact" staged-artifact
  if ! (cd -- "$staging_dir" && sha256sum -c -- SHA256SUMS); then
    die 'staged SHA256SUMS verification failed'
  fi
  if ! rm -f -- "$staged_compare_one" "$staged_compare_two"; then
    die 'could not remove temporary staged comparison files'
  fi
  [[ ! -e $staged_compare_one && ! -e $staged_compare_two ]] || \
    die 'temporary staged comparison files remain'

  if ((output_dir_new == 0)); then
    if ! rmdir -- "$output_dir"; then
      die 'could not remove the empty output directory before publication'
    fi
  fi
  [[ ! -e $output_dir && ! -L $output_dir ]] || \
    die 'output destination appeared before publication'

  artifact_path="$output_dir/$artifact_base"
  checksums_path="$output_dir/SHA256SUMS"
  build_info_path="$output_dir/BUILD_INFO.txt"
  if ! mv -T --no-clobber -- "$staging_dir" "$output_dir"; then
    die 'could not publish the complete output bundle'
  fi
  if [[ -e $staging_dir || -L $staging_dir ]]; then
    die 'output destination already existed; refusing to overwrite it'
  fi
  bundle_published=1
  if ! current_identity=$(stat -c '%d:%i' -- "$output_dir"); then
    die 'could not record the published bundle identity'
  fi
  published_identity=$current_identity

  printf 'artifact directory: %s\n' "$output_dir"
  printf 'artifact: %s\n' "$artifact_path"
  printf 'checksums: %s\n' "$checksums_path"
  printf 'build info: %s\n' "$build_info_path"
}

if ! cmp -s "$binary_one" "$binary_two"; then
  printf 'error: reproducible build mismatch\n' >&2
  printf 'fluxos (build 1) SHA-256: %s\n' "$(sha256_of "$binary_one")" >&2
  printf 'fluxos (build 2) SHA-256: %s\n' "$(sha256_of "$binary_two")" >&2
  exit 1
fi

artifact_sha=$(sha256_of "$binary_one")
verify_worktree_state
promote_output
verify_worktree_state

printf 'fluxos SHA-256: %s\n' "$artifact_sha"

printf 'result: reproducible\n'
