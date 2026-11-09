#!/bin/sh
# What CI checks before the Catalog is published, with no private key: that
# catalog/catalog.json is signed with the key in keys/catalog.pub (as
# Laneway checks it) and is exactly what the Plugin releases build to.
#
#	scripts/check-catalog.sh [releases-dir]
set -eu
cd "$(dirname "$0")/.."

if [ ! -f catalog/catalog.json ]; then
	echo "There is no Catalog yet (catalog/catalog.json); nothing to check."
	exit 0
fi
go run ./cmd/plugins verify catalog/catalog.json
rebuilt=$(mktemp)
trap 'rm -f "$rebuilt"' EXIT
scripts/build-catalog.sh -o "$rebuilt" ${1:-} >/dev/null
if ! cmp -s "$rebuilt" catalog/catalog.json; then
	echo "catalog/catalog.json isn't what the releases build to:" >&2
	diff -u catalog/catalog.json "$rebuilt" >&2 || true
	exit 1
fi
echo "catalog/catalog.json is what the releases build to."
