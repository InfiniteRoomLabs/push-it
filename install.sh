#!/bin/sh
# push-it bootstrap: download the latest release for this machine, verify its
# checksum, put the binary in ~/.local/bin, then run `push-it install "$@"`.
#
#   curl -fsSL https://raw.githubusercontent.com/InfiniteRoomLabs/push-it/main/install.sh | sh -s -- --all
#
# Env overrides: PUSH_IT_VERSION (default latest), PUSH_IT_BIN_DIR
# (default ~/.local/bin), PUSH_IT_BASE_URL (release asset base URL).
set -eu

version=${PUSH_IT_VERSION:-latest}
bin_dir=${PUSH_IT_BIN_DIR:-$HOME/.local/bin}
if [ -n "${PUSH_IT_BASE_URL:-}" ]; then
  base=$PUSH_IT_BASE_URL
elif [ "$version" = latest ]; then
  base=https://github.com/InfiniteRoomLabs/push-it/releases/latest/download
else
  base=https://github.com/InfiniteRoomLabs/push-it/releases/download/$version
fi

os=$(uname -s | tr '[:upper:]' '[:lower:]')
case $os in
  linux|darwin) ;;
  *) echo "install.sh: unsupported OS '$os' (on Windows, download the zip from the releases page)" >&2; exit 1 ;;
esac
case $(uname -m) in
  x86_64|amd64) arch=amd64 ;;
  aarch64|arm64) arch=arm64 ;;
  *) echo "install.sh: unsupported architecture '$(uname -m)'" >&2; exit 1 ;;
esac
asset="push-it_${os}_${arch}.tar.gz"

fetch() {
  if command -v curl >/dev/null 2>&1; then
    curl -fsSL -o "$2" "$1"
  elif command -v wget >/dev/null 2>&1; then
    wget -qO "$2" "$1"
  else
    echo "install.sh: need curl or wget" >&2; exit 1
  fi
}

sha256() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$1" | cut -d' ' -f1
  elif command -v shasum >/dev/null 2>&1; then
    shasum -a 256 "$1" | cut -d' ' -f1
  else
    echo "install.sh: need sha256sum or shasum" >&2; exit 1
  fi
}

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
echo "downloading $base/$asset"
fetch "$base/$asset" "$tmp/$asset"
fetch "$base/checksums.txt" "$tmp/checksums.txt"

want=$(grep " $asset\$" "$tmp/checksums.txt" | cut -d' ' -f1)
[ -n "$want" ] || { echo "install.sh: $asset not listed in checksums.txt" >&2; exit 1; }
got=$(sha256 "$tmp/$asset")
[ "$got" = "$want" ] || { echo "install.sh: checksum mismatch for $asset (got $got, want $want)" >&2; exit 1; }

tar -C "$tmp" -xzf "$tmp/$asset" push-it
mkdir -p "$bin_dir"
mv "$tmp/push-it" "$bin_dir/push-it"
chmod +x "$bin_dir/push-it"
echo "installed $bin_dir/push-it"
case ":$PATH:" in
  *":$bin_dir:"*) ;;
  *) echo "note: $bin_dir is not on your PATH" >&2 ;;
esac

exec "$bin_dir/push-it" install "$@"
