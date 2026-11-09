#!/bin/sh
# Rebuilds the Catalog, catalog/catalog.json (or the file given as -o), from
# the Plugin releases: those in a directory, or else every release on GitHub
# (see fetch-releases.sh), listed at $BASE_URL (GitHub's release downloads
# by default). The Catalog it writes is unsigned: the Catalog workflow
# signs it in CI, or sign-catalog.sh by hand.
#
#	scripts/build-catalog.sh [-o file] [releases-dir]
set -eu
cd "$(dirname "$0")/.."

out=catalog/catalog.json
if [ "${1:-}" = -o ]; then
	out=$2
	shift 2
fi
releases=${1:-}
if [ -z "$releases" ]; then
	releases=$(mktemp -d)
	trap 'rm -rf "$releases"' EXIT
	scripts/fetch-releases.sh "$releases"
fi
${PLUGINS:-go run ./cmd/plugins} catalog -releases "$releases" -o "$out" ${BASE_URL:+-base-url "$BASE_URL"}
