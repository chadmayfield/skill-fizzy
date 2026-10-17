#!/bin/sh
# Checks a release tag before the release workflow builds anything.
# Reads GITHUB_REF_NAME (the tag) and GITHUB_SHA (the tagged commit).
# Prints "release" or "prerelease" and exits 0 when the tag may be published.
# Prints the reason on stderr and exits 1 when it may not.
# Run it inside the checkout, after "git fetch origin main".
set -eu

tag="${GITHUB_REF_NAME:?GITHUB_REF_NAME is not set}"
sha="${GITHUB_SHA:?GITHUB_SHA is not set}"

if ! printf '%s\n' "$tag" | grep -Eq '^v[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?$'; then
	echo "release-guard: tag '$tag' is not vMAJOR.MINOR.PATCH or vMAJOR.MINOR.PATCH-suffix" >&2
	exit 1
fi

if ! git merge-base --is-ancestor "$sha" origin/main; then
	echo "release-guard: tag '$tag' ($sha) is not on origin/main" >&2
	exit 1
fi

case "$tag" in
*-*) echo prerelease ;;
*) echo release ;;
esac
