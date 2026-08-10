#!/bin/sh
set -eu

ROOT=$(CDPATH='' cd -- "$(dirname -- "$0")/.." && pwd)
TMPDIR=${TMPDIR:-/tmp}
work=$(mktemp -d "$TMPDIR/hitch-install-test.XXXXXX")
trap 'rm -rf "$work"' EXIT INT TERM

assert_eq() {
  actual=$1
  expected=$2
  message=$3
  if [ "$actual" != "$expected" ]; then
    printf 'not ok: %s\nexpected: %s\nactual: %s\n' "$message" "$expected" "$actual" >&2
    exit 1
  fi
}

HITCH_INSTALL_SOURCE_ONLY=1 . "$ROOT/install.sh"

assert_eq "$(asset_name darwin amd64)" "hitch_darwin_amd64.tar.gz" "darwin amd64 asset name"
assert_eq "$(asset_name darwin arm64)" "hitch_darwin_arm64.tar.gz" "darwin arm64 asset name"
assert_eq "$(asset_name linux amd64)" "hitch_linux_amd64.tar.gz" "linux amd64 asset name"
assert_eq "$(asset_name linux arm64)" "hitch_linux_arm64.tar.gz" "linux arm64 asset name"

printf 'payload' > "$work/hitch_linux_amd64.tar.gz"
checksum=$(sha256_file "$work/hitch_linux_amd64.tar.gz")
printf '%s  hitch_linux_amd64.tar.gz\n' "$checksum" > "$work/checksums.txt"
verify_checksum "$work/hitch_linux_amd64.tar.gz" "$work/checksums.txt" "hitch_linux_amd64.tar.gz"

printf '0000000000000000000000000000000000000000000000000000000000000000  hitch_linux_amd64.tar.gz\n' > "$work/bad-checksums.txt"
set +e
HITCH_INSTALL_SOURCE_ONLY=1 sh -c '. "$1"; verify_checksum "$2" "$3" hitch_linux_amd64.tar.gz' sh "$ROOT/install.sh" "$work/hitch_linux_amd64.tar.gz" "$work/bad-checksums.txt" 2>"$work/mismatch.err"
status=$?
set -e
if [ "$status" -eq 0 ]; then
  printf 'not ok: checksum mismatch exited zero\n' >&2
  exit 1
fi
if ! grep -q 'checksum mismatch for hitch_linux_amd64.tar.gz' "$work/mismatch.err"; then
  printf 'not ok: checksum mismatch did not print actionable error\n' >&2
  exit 1
fi

set +e
asset_name freebsd amd64 >/dev/null 2>"$work/platform.err"
status=$?
set -e
if [ "$status" -eq 0 ]; then
  printf 'not ok: unsupported platform exited zero\n' >&2
  exit 1
fi

printf 'ok install.sh\n'
