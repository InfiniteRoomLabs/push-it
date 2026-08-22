#!/bin/sh
# Self-check for scripts/changelog-notes.sh against a fixture changelog.
set -eu
cd "$(dirname "$0")/.."
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
cat > "$tmp/CHANGELOG.md" <<'EOF'
# Changelog

## [Unreleased]

### Added

- not released yet

## [0.2.0] - 2026-09-01

### Added

- second thing

### Fixed

- a fix

## [0.1.0] - 2026-08-22

### Added

- first thing

[Unreleased]: https://github.com/InfiniteRoomLabs/push-it/compare/v0.2.0...HEAD
[0.2.0]: https://github.com/InfiniteRoomLabs/push-it/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/InfiniteRoomLabs/push-it/releases/tag/v0.1.0
EOF

got=$(CHANGELOG=$tmp/CHANGELOG.md sh scripts/changelog-notes.sh v0.2.0)
want='### Added

- second thing

### Fixed

- a fix'
[ "$got" = "$want" ] || { printf 'v0.2.0: got:\n%s\nwant:\n%s\n' "$got" "$want"; exit 1; }

got=$(CHANGELOG=$tmp/CHANGELOG.md sh scripts/changelog-notes.sh v0.1.0)
[ "$got" = '### Added

- first thing' ] || { printf 'v0.1.0: got:\n%s\n' "$got"; exit 1; }

if CHANGELOG=$tmp/CHANGELOG.md sh scripts/changelog-notes.sh v9.9.9 >/dev/null 2>&1; then
  echo "expected failure for missing version"; exit 1
fi
if sh scripts/changelog-notes.sh 0.1.0 >/dev/null 2>&1; then
  echo "expected failure for tag without v prefix"; exit 1
fi
echo "changelog-notes: ok"
