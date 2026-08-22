#!/bin/sh
# Exercises install.sh end to end against a fake release served over file://.
set -eu
cd "$(dirname "$0")/.."
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
os=$(uname -s | tr '[:upper:]' '[:lower:]')
case $(uname -m) in
  x86_64|amd64) arch=amd64 ;;
  aarch64|arm64) arch=arm64 ;;
  *) echo "unsupported test arch"; exit 1 ;;
esac

# fake release: a tarball whose push-it is a script that echoes its argv
mkdir -p "$tmp/rel" "$tmp/pkg"
printf '#!/bin/sh\necho "fake push-it: $*"\n' > "$tmp/pkg/push-it"
chmod +x "$tmp/pkg/push-it"
asset="push-it_${os}_${arch}.tar.gz"
tar -C "$tmp/pkg" -czf "$tmp/rel/$asset" push-it
(cd "$tmp/rel" && sha256sum "$asset" > checksums.txt)

# happy path: installs, then execs `push-it install "$@"`
out=$(PUSH_IT_BASE_URL="file://$tmp/rel" PUSH_IT_BIN_DIR="$tmp/bin" sh install.sh --sound --yes)
case $out in
  *"fake push-it: install --sound --yes"*) ;;
  *) printf 'unexpected output:\n%s\n' "$out"; exit 1 ;;
esac
[ -x "$tmp/bin/push-it" ] || { echo "binary not installed"; exit 1; }

# tampered tarball must be refused
printf 'garbage' >> "$tmp/rel/$asset"
if PUSH_IT_BASE_URL="file://$tmp/rel" PUSH_IT_BIN_DIR="$tmp/bin2" sh install.sh --yes >/dev/null 2>&1; then
  echo "expected checksum failure"; exit 1
fi
[ ! -e "$tmp/bin2/push-it" ] || { echo "tampered binary was installed"; exit 1; }

echo "install.sh: ok"
