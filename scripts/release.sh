#!/bin/sh
# Tags a release after checking the tree is clean, on main, and the changelog
# has a section for it. Pushes the tag to both remotes; release.yml does the rest.
set -eu
cd "$(dirname "$0")/.."
tag=${1:-}
case $tag in
  v[0-9]*.[0-9]*.[0-9]*) ;;
  *) echo "usage: release.sh vX.Y.Z" >&2; exit 1 ;;
esac
[ "$(git branch --show-current)" = main ] || { echo "release.sh: not on main" >&2; exit 1; }
[ -z "$(git status --porcelain)" ] || { echo "release.sh: working tree not clean" >&2; exit 1; }
git rev-parse -q --verify "refs/tags/$tag" >/dev/null && { echo "release.sh: $tag already exists" >&2; exit 1; }
sh scripts/changelog-notes.sh "$tag" >/dev/null
git tag -a "$tag" -m "$tag"
for r in origin github; do
  git remote get-url "$r" >/dev/null 2>&1 && git push "$r" "$tag"
done
echo "tagged and pushed $tag - watch https://github.com/InfiniteRoomLabs/push-it/actions"
