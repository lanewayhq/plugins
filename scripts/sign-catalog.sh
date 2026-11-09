#!/bin/sh
# Signs the Catalog with its private key. Run it on the machine that holds
# the key, offline from CI: CI never sees the key, only checks the
# signature against keys/catalog.pub.
#
#	scripts/sign-catalog.sh <private-key-file> [releases-dir]
#
# It first rebuilds the Catalog from the Plugin releases (those in
# releases-dir, or else every release on GitHub, which needs the GitHub
# CLI) and refuses to sign catalog/catalog.json unless it is exactly that,
# so what gets signed is what the releases are, whoever wrote the file. Then
# it writes catalog/catalog.json.sig and checks it against the public key
# ($CATALOG_PUB, keys/catalog.pub by default). Commit both files.
set -eu
cd "$(dirname "$0")/.."

key=${1:?usage: scripts/sign-catalog.sh <private-key-file> [releases-dir]}
pub=${CATALOG_PUB:-keys/catalog.pub}
releases=${2:-}

rebuilt=$(mktemp)
trap 'rm -f "$rebuilt"' EXIT
scripts/build-catalog.sh -o "$rebuilt" $releases >/dev/null
if ! cmp -s "$rebuilt" catalog/catalog.json; then
	echo "catalog/catalog.json isn't what the releases build to; not signing it. The difference:" >&2
	diff -u catalog/catalog.json "$rebuilt" >&2 || true
	echo "Run scripts/build-catalog.sh${releases:+ $releases}, review the change, and sign again." >&2
	exit 1
fi
${PLUGINS:-go run ./cmd/plugins} sign -key "$key" catalog/catalog.json
${PLUGINS:-go run ./cmd/plugins} verify -pub "$pub" catalog/catalog.json
echo "Now commit catalog/catalog.json and catalog/catalog.json.sig."
