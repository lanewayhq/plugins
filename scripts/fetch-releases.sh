#!/bin/sh
# Downloads the archive of every Plugin release (every release tagged
# <plugin>/v<version>) into a directory, one subdirectory per tag, for
# building the Catalog. Needs the GitHub CLI, signed in to an account that
# can read the repository ($REPO, lanewayhq/plugins by default).
#
#	scripts/fetch-releases.sh <dir>
set -eu

dir=${1:?usage: scripts/fetch-releases.sh <dir>}
repo=${REPO:-lanewayhq/plugins}

mkdir -p "$dir"
gh release list --repo "$repo" --exclude-drafts --limit 1000 --json tagName --jq '.[].tagName' |
	grep -E '^[a-z][a-z0-9-]*/v[0-9]' |
	grep -v '^catalog/' |
	while read -r tag; do
		mkdir -p "$dir/$tag"
		gh release download "$tag" --repo "$repo" --pattern '*.tar.gz' --dir "$dir/$tag" --clobber
	done
