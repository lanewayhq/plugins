# The Catalog signing key

Laneway refuses a Catalog unless it is signed with the Ed25519 key built into it (`placeholderKey` in [lanewayhq/laneway's `internal/catalog/fetch.go`](https://github.com/lanewayhq/laneway/blob/main/internal/catalog/fetch.go)). [`catalog.pub`](catalog.pub) is the public half of that key: CI checks the Catalog's signature against it before publishing. The private half is kept **offline**. It is never in this repository, in CI secrets, or on a machine that runs CI, so taking over this repository or its releases isn't enough to get code onto a developer's machine.

`catalog.pub` holds a **placeholder** until a maintainer does the steps below. While it does, nothing can sign a Catalog that verifies, so none is published.

## Generating the key (once)

On a machine you trust, ideally offline, with Go and this repository:

```sh
go run ./cmd/plugins keygen -out /path/to/offline/storage
```

It writes two files and prints the public key:

- `catalog.key`: the private key, a base64 Ed25519 seed, readable only by you. Keep it on offline storage (an encrypted USB drive, or an offline password-manager vault), and keep a second copy somewhere separate: losing it means shipping a Laneway release with a new key before another Catalog can be published.
- `catalog.pub`: the public key, one base64 line.

Then, in two pull requests:

1. **Here:** replace [`catalog.pub`](catalog.pub) with the new `catalog.pub`, dropping the PLACEHOLDER comment.
2. **In lanewayhq/laneway:** set `placeholderKey` in `internal/catalog/fetch.go` to the same base64 line, and remove the PLACEHOLDER notes there (and on `DefaultURL` once the Catalog is published). Every Laneway built from then on accepts only Catalogs signed with this key.

Never commit `catalog.key`, paste it into a CI secret, or copy it to a machine that runs CI.

## Signing

See [docs/releasing.md](../docs/releasing.md#signing-the-catalog). In short, on the machine with the key:

```sh
scripts/sign-catalog.sh /path/to/offline/storage/catalog.key
```

## Replacing the key

A new key needs a Laneway release that builds it in, and Laneways built before then refuse Catalogs signed with it. Generate the new pair as above, ship it in Laneway first, then replace `catalog.pub` here and sign the Catalog with the new key.
