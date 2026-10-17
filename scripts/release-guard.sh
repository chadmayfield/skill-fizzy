#!/bin/sh
# Checks a release tag before the release workflow builds anything.
# Reads GITHUB_REF_NAME (the tag) and GITHUB_SHA (the tagged commit).
# On success it prints "release" or "prerelease", appends "kind=<that>" to
# the file named by GITHUB_OUTPUT when that variable is set, and exits 0.
# On any rejection it prints the reason on stderr, writes nothing to
# GITHUB_OUTPUT and exits 1.
# Run it inside the checkout, after "git fetch origin main".
set -eu

fail() {
	echo "release-guard: $1" >&2
	exit 1
}

tag="${GITHUB_REF_NAME:?GITHUB_REF_NAME is not set}"
sha="${GITHUB_SHA:?GITHUB_SHA is not set}"

nl=$(printf '\n.')
nl=${nl%.}
cr=$(printf '\r')
case "$tag" in
*"$nl"* | *"$cr"*) fail "tag has a line break" ;;
esac
case "$tag" in
v*) ;;
*) fail "tag '$tag' does not start with v" ;;
esac
case "$tag" in
*[!0-9A-Za-z.v-]*) fail "tag '$tag' has a character outside 0-9, A-Z, a-z, '.' and '-'" ;;
esac

rest=${tag#v}
core=$rest
pre=
kind=release
case "$rest" in
*-*)
	core=${rest%%-*}
	pre=${rest#*-}
	kind=prerelease
	[ -n "$pre" ] || fail "tag '$tag' has an empty suffix"
	;;
esac

# core must be MAJOR.MINOR.PATCH, each part a number without a leading zero.
parts=0
remaining=$core.
while [ -n "$remaining" ]; do
	p=${remaining%%.*}
	remaining=${remaining#*.}
	parts=$((parts + 1))
	case "$p" in
	0) ;;
	[1-9]*)
		case "$p" in
		*[!0-9]*) fail "tag '$tag' has a non-numeric version part" ;;
		esac
		;;
	*) fail "tag '$tag' has an empty, non-numeric or zero-padded version part" ;;
	esac
done
[ "$parts" -eq 3 ] || fail "tag '$tag' is not vMAJOR.MINOR.PATCH"

git merge-base --is-ancestor "$sha" origin/main || fail "tag '$tag' ($sha) is not on origin/main"

echo "$kind"
if [ -n "${GITHUB_OUTPUT:-}" ]; then
	echo "kind=$kind" >>"$GITHUB_OUTPUT"
fi
