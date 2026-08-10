#!/bin/sh
set -eu

REPO=${HITCH_REPO:-artisan-build/hitch}
VERSION=${HITCH_VERSION:-latest}
PREFIX=${PREFIX:-/usr/local}
HITCH_TMP_BIN=

say() { printf '%s\n' "$*"; }
fail() { printf 'hitch install: %s\n' "$*" >&2; exit 1; }

detect_os() {
  os=$(uname -s 2>/dev/null || true)
  case "$os" in
    Darwin) printf 'darwin' ;;
    Linux) printf 'linux' ;;
    *) return 1 ;;
  esac
}

detect_arch() {
  arch=$(uname -m 2>/dev/null || true)
  case "$arch" in
    x86_64 | amd64) printf 'amd64' ;;
    arm64 | aarch64) printf 'arm64' ;;
    *) return 1 ;;
  esac
}

asset_name() {
  os=$1
  arch=$2
  case "$os/$arch" in
    darwin/amd64 | darwin/arm64 | linux/amd64 | linux/arm64)
      printf 'hitch_%s_%s.tar.gz' "$os" "$arch"
      ;;
    *) return 1 ;;
  esac
}

release_base_url() {
  if [ "$VERSION" = latest ]; then
    printf 'https://github.com/%s/releases/latest/download' "$REPO"
  else
    printf 'https://github.com/%s/releases/download/%s' "$REPO" "$VERSION"
  fi
}

choose_install_dir() {
  if [ -n "${INSTALL_DIR:-}" ]; then
    printf '%s' "$INSTALL_DIR"
    return 0
  fi
  if mkdir -p "$PREFIX/bin" 2>/dev/null && [ -w "$PREFIX/bin" ]; then
    printf '%s/bin' "$PREFIX"
    return 0
  fi
  user_bin=${HOME:-}/.local/bin
  if [ -n "${HOME:-}" ] && mkdir -p "$user_bin" 2>/dev/null && [ -w "$user_bin" ]; then
    printf '%s' "$user_bin"
    return 0
  fi
  return 1
}

download_file() {
  url=$1
  dest=$2
  if command -v curl >/dev/null 2>&1; then
    curl -fsSL "$url" -o "$dest"
    return $?
  fi
  if command -v wget >/dev/null 2>&1; then
    wget -q "$url" -O "$dest"
    return $?
  fi
  fail "curl or wget is required to download releases"
}

sha256_file() {
  file=$1
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$file" | cut -d ' ' -f 1
    return 0
  fi
  if command -v shasum >/dev/null 2>&1; then
    shasum -a 256 "$file" | cut -d ' ' -f 1
    return 0
  fi
  fail "sha256sum or shasum is required to verify downloads"
}

expected_checksum() {
  checksums=$1
  asset=$2
  awk -v asset="$asset" '$2 == asset { print $1 }' "$checksums"
}

verify_checksum() {
  file=$1
  checksums=$2
  asset=$3
  expected=$(expected_checksum "$checksums" "$asset")
  [ -n "$expected" ] || fail "checksums.txt does not contain $asset"
  actual=$(sha256_file "$file")
  [ "$actual" != "$expected" ] || fail "checksum mismatch for $asset"
}

fail_install_archive() {
  tmp_bin=$1
  message=$2
  rm -f "$tmp_bin"
  HITCH_TMP_BIN=
  fail "$message"
}

install_archive() {
  archive=$1
  dir=$2
  tmp_extract=$3
  mkdir -p "$tmp_extract" || fail "failed to prepare temporary extraction directory"
  tar -xzf "$archive" -C "$tmp_extract" || fail "failed to extract release archive"
  [ -f "$tmp_extract/hitch" ] || fail "release archive does not contain hitch"
  chmod 0755 "$tmp_extract/hitch" || fail "failed to mark hitch executable"
  tmp_bin=$(mktemp "$dir/.hitch.XXXXXX") || fail "failed to create a temporary install file in $dir"
  HITCH_TMP_BIN=$tmp_bin
  cp "$tmp_extract/hitch" "$tmp_bin" || fail_install_archive "$tmp_bin" "failed to copy hitch into $dir"
  chmod 0755 "$tmp_bin" || fail_install_archive "$tmp_bin" "failed to mark $tmp_bin executable"
  mv "$tmp_bin" "$dir/hitch" || fail_install_archive "$tmp_bin" "failed to install hitch to $dir/hitch"
  HITCH_TMP_BIN=
}

main() {
  os=$(detect_os) || fail "unsupported operating system: $(uname -s 2>/dev/null || printf unknown)"
  arch=$(detect_arch) || fail "unsupported architecture: $(uname -m 2>/dev/null || printf unknown)"
  asset=$(asset_name "$os" "$arch") || fail "unsupported platform: $os/$arch"
  base=$(release_base_url)
  install_dir=$(choose_install_dir) || fail "no writable install directory; set INSTALL_DIR or PREFIX"
  tmp_dir=$(mktemp -d 2>/dev/null || mktemp -d -t hitch)
  trap 'rm -rf "$tmp_dir"; [ -z "$HITCH_TMP_BIN" ] || rm -f "$HITCH_TMP_BIN"' EXIT INT TERM

  say "Installing hitch ${VERSION} for ${os}/${arch}"
  say "Downloading ${asset}"
  download_file "$base/$asset" "$tmp_dir/$asset" || fail "no such release or asset for ${VERSION}: failed to download $asset from $base"
  say "Downloading checksums.txt"
  download_file "$base/checksums.txt" "$tmp_dir/checksums.txt" || fail "no such release or checksums for ${VERSION}: failed to download checksums.txt from $base"
  say "Verifying checksum"
  verify_checksum "$tmp_dir/$asset" "$tmp_dir/checksums.txt" "$asset"
  say "Installing to ${install_dir}/hitch"
  install_archive "$tmp_dir/$asset" "$install_dir" "$tmp_dir/extract"
  say "hitch installed at ${install_dir}/hitch"

  case ":$PATH:" in
    *:"$install_dir":*) ;;
    *) say "Add ${install_dir} to PATH to run hitch without a full path." ;;
  esac
}

if [ "${HITCH_INSTALL_SOURCE_ONLY:-}" != 1 ]; then
  main "$@"
fi
