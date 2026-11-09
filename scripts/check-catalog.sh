#!/bin/sh
# What CI checks before the Catalog is signed and published, with no private
# key: that catalog/catalog.json is exactly what the Plugin releases build
# to, and, if a signature is committed beside it, that it is signed with the
# key in keys/catalog.pub (as Laneway checks it).
#
#	scripts/check-catalog.sh [releases-dir]
set -eu
cd "$(dirname "$0")/.."

if [ ! -f catalog/catalog.json ]; then
	echo "There is no Catalog yet (catalog/catalog.json); nothing to check."
	exit 0
fi
if [ -f catalog/catalog.json.sig ]; then
	${PLUGINS:-go run ./cmd/plugins} verify catalog/catalog.json
fi
rebuilt=$(mktemp)
trap 'rm -f "$rebuilt"' EXIT
scripts/build-catalog.sh -o "$rebuilt" ${1:-} >/dev/null
if ! cmp -s "$rebuilt" catalog/catalog.json; then
	echo "catalog/catalog.json isn't what the releases build to:" >&2
	diff -u catalog/catalog.json "$rebuilt" >&2 || true
	exit 1
fi
echo "catalog/catalog.json is what the releases build to."
