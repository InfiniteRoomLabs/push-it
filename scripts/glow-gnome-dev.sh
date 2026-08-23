#!/bin/sh
# shellcheck disable=SC2016
# Nested GNOME Shell dev loop for the glow extension: run the extension from
# the working tree without logging out. Everything (extension dir, dconf
# database, push-it config) lives in a throwaway temp dir, so the real
# session's enabled-extensions list and config are never touched.
#
# Env knobs: PUSH_IT_DEV_SHELL_WAIT (seconds before triggering, default 4),
# PUSH_IT_DEV_GLOW_DURATION (default 3s), MUTTER_DEBUG_DUMMY_MODE_SPECS
# (nested window size, default 1280x720).
set -eu
cd "$(dirname "$0")/.."
for cmd in gnome-shell dbus-run-session gnome-extensions; do
  command -v "$cmd" >/dev/null 2>&1 || { echo "glow-gnome-dev: $cmd not found" >&2; exit 1; }
done
mise run build
tmp=$(mktemp -d 2>/dev/null || mktemp -d -t push-it-glow-dev)
trap 'rm -rf "$tmp"' EXIT
trap 'exit 130' INT HUP TERM
uuid="pushit-glow@infiniteroomlabs.com"
extdir="$tmp/data/gnome-shell/extensions/$uuid"
mkdir -p "$extdir" "$tmp/config" "$tmp/confighome"
cp internal/glow/gnome/ext/* "$extdir/"

XDG_DATA_HOME="$tmp/data" \
XDG_CONFIG_HOME="$tmp/confighome" \
PUSH_IT_CONFIG_DIR="$tmp/config" \
XDG_CURRENT_DESKTOP=GNOME \
MUTTER_DEBUG_DUMMY_MODE_SPECS="${MUTTER_DEBUG_DUMMY_MODE_SPECS:-1280x720}" \
PUSH_IT_DEV_SHELL_WAIT="${PUSH_IT_DEV_SHELL_WAIT:-4}" \
PUSH_IT_DEV_GLOW_DURATION="${PUSH_IT_DEV_GLOW_DURATION:-3s}" \
dbus-run-session -- sh -c '
  gnome-shell --nested --wayland &
  shell=$!
  sleep "$PUSH_IT_DEV_SHELL_WAIT"
  gnome-extensions enable "'"$uuid"'" || true
  if ./bin/push-it glow --duration "$PUSH_IT_DEV_GLOW_DURATION"; then
    echo "glow-gnome-dev: glow triggered; close the nested Shell window (or Ctrl+C) to exit"
  else
    echo "glow-gnome-dev: glow FAILED (see error above); close the nested Shell window (or Ctrl+C) to exit"
  fi
  wait $shell
'
