#!/bin/sh
# Signs the Catalog by hand, for when CI can't (the Catalog workflow signs
# it in CI once the maintainer approves). Run it on the machine that holds
# the private key.
#
#	scripts/sign-catalog.sh <private-key-file> [releases-dir]
#
# It rebuilds catalog/catalog.json from the Plugin releases (those in
# releases-dir, or else every release on GitHub, which needs the GitHub
# CLI), so what gets signed is what the releases are. Then it writes
# catalog/catalog.json.sig, checks it against the public key ($CATALOG_PUB,
# keys/catalog.pub by default), and prints how to publish the two files.
set -eu
cd "$(dirname "$0")/.."

key=${1:?usage: scripts/sign-catalog.sh <private-key-file> [releases-dir]}
pub=${CATALOG_PUB:-keys/catalog.pub}

scripts/build-catalog.sh ${2:-}
${PLUGINS:-go run ./cmd/plugins} sign -key "$key" catalog/catalog.json
${PLUGINS:-go run ./cmd/plugins} verify -pub "$pub" catalog/catalog.json
echo "Review catalog/catalog.json, then publish it as the latest release:"
echo "  gh release create catalog/\$(date -u +%Y%m%d%H%M%S) catalog/catalog.json catalog/catalog.json.sig --latest --title \"Catalog \$(date -u +%Y-%m-%d)\""
