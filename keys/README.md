# The Catalog signing key

Laneway refuses a Catalog unless it is signed with the Ed25519 key built into it (`catalogKey` in [lanewayhq/laneway's `internal/catalog/fetch.go`](https://github.com/lanewayhq/laneway/blob/main/internal/catalog/fetch.go)). [`catalog.pub`](catalog.pub) is the public half of that key. The private half is the `CATALOG_SIGNING_KEY` secret of this repository's `catalog-signing` Environment, which only a job running on `main` can read, and only after the maintainer approves that run, plus a backup copy in the maintainer's password manager. It is never in this repository. Whoever holds the private half can get code installed on every Laneway machine, and a leaked key can't be taken back from Laneways already installed, so guard it and the GitHub account that approves signing.

## Generating the key (once)

With Go, the GitHub CLI and this repository:

```sh
go run ./cmd/plugins keygen -out ~/laneway-catalog-key
gh secret set CATALOG_SIGNING_KEY --env catalog-signing -R lanewayhq/plugins < ~/laneway-catalog-key/catalog.key
```

Then copy `~/laneway-catalog-key/catalog.key` into your password manager as a backup (losing every copy means shipping a Laneway release with a new key before another Catalog can be published) and delete it: `rm ~/laneway-catalog-key/catalog.key`.

`~/laneway-catalog-key/catalog.pub` is the public half, one base64 line. Put it in two places:

1. **Here:** replace [`catalog.pub`](catalog.pub) with it.
2. **In lanewayhq/laneway:** set `catalogKey` in `internal/catalog/fetch.go` to the same line. Every Laneway built from then on accepts only Catalogs signed with this key.

## Signing

CI signs after you approve; see [docs/releasing.md](../docs/releasing.md#signing-the-catalog). To sign by hand instead: `scripts/sign-catalog.sh <key-file>`.

## Replacing the key

A new key needs a Laneway release that builds it in, and Laneways built before then refuse Catalogs signed with it. Generate the new pair as above, ship it in Laneway first, then replace `catalog.pub` here and sign the Catalog with the new key.
