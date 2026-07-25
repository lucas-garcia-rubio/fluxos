#!/usr/bin/env bash

set -euo pipefail
umask 077

usage() {
  printf 'usage: %s [--diagnostic] <version>\n' "${0##*/}" >&2
  exit 2
}

die() {
  printf 'error: %s\n' "$*" >&2
  exit 1
}

warning() {
  printf 'warning: %s\n' "$*" >&2
}

if [[ $# -eq 1 ]]; then
  diagnostic=0
  version=$1
elif [[ $# -eq 2 && $1 == '--diagnostic' ]]; then
  diagnostic=1
  version=$2
else
  usage
fi

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
  warning "this is a diagnostic-only proof on a non-baseline host (${host_issues[*]}); it is never an official qualification"
elif ((diagnostic == 1)); then
  warning '--diagnostic was requested; this run is diagnostic-only and never an official qualification'
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
cleanup() {
  rm -rf -- "$temporary_root" || true
}
trap cleanup EXIT

if ! cache_one=$(mktemp -d "$temporary_root/cache-one.XXXXXXXX"); then
  die 'could not create the first temporary GOCACHE'
fi
if ! cache_two=$(mktemp -d "$temporary_root/cache-two.XXXXXXXX"); then
  die 'could not create the second temporary GOCACHE'
fi
output_dir="$temporary_root/bin"
if ! mkdir -- "$output_dir"; then
  die 'could not create the temporary binary directory'
fi

binary_one="$output_dir/fluxos-build-one"
binary_two="$output_dir/fluxos-build-two"

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

if ! cmp -s "$binary_one" "$binary_two"; then
  printf 'error: reproducible build mismatch\n' >&2
  printf 'fluxos (build 1) SHA-256: %s\n' "$(sha256_of "$binary_one")" >&2
  printf 'fluxos (build 2) SHA-256: %s\n' "$(sha256_of "$binary_two")" >&2
  exit 1
fi

printf 'fluxos SHA-256: %s\n' "$(sha256_of "$binary_one")"

if ! final_head=$(git -C "$git_root" rev-parse --verify HEAD); then
  die 'could not resolve HEAD after the build'
fi
[[ $final_head == "$initial_head" ]] || die 'HEAD changed during verification'
if ! final_status=$(git -C "$git_root" status --porcelain=v1 --untracked-files=all); then
  die 'could not inspect the Git worktree after the build'
fi
[[ -z $final_status ]] || die 'Git worktree is no longer clean after the build'

printf 'result: reproducible\n'
