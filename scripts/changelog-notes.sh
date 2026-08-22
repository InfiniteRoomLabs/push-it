#!/bin/sh
# Prints the CHANGELOG.md section for a release tag (vX.Y.Z) to stdout.
# Exits 1 if the tag is malformed or the section is missing.
set -eu
tag=${1:-}
case $tag in
  v[0-9]*.[0-9]*.[0-9]*) ;;
  *) echo "usage: changelog-notes.sh vX.Y.Z" >&2; exit 1 ;;
esac
ver=${tag#v}
file=${CHANGELOG:-CHANGELOG.md}
notes=$(awk -v ver="$ver" '
  /^## \[/ { if (found) exit; found = ($0 ~ "^## \\[" ver "\\]") ; next }
  found { print }
' "$file")
# trim leading/trailing blank lines
notes=$(printf '%s\n' "$notes" | sed -e '/./,$!d' | sed -e ':a' -e '/^\n*$/{$d;N;ba' -e '}')
if [ -z "$notes" ]; then
  echo "changelog-notes.sh: no '## [$ver]' section in $file" >&2
  exit 1
fi
printf '%s\n' "$notes"
