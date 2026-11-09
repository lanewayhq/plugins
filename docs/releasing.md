# Releasing Plugins and the Catalog

Each first-party Plugin is released on its own, by a tag. The Catalog, the signed index Laneway fetches Plugins from, is rebuilt from every release and signed with a key kept offline: CI never holds it.

```
tag laravel/v1.2.0 ──▶ Release workflow: test against Laneway, publish the release
                         └─▶ rebuild the Catalog, open a "Catalog: laravel 1.2.0" PR (unsigned)
maintainer, offline ──▶ scripts/sign-catalog.sh <key>, push catalog.json.sig to the PR
Catalog workflow    ──▶ on the PR: verify the signature against keys/catalog.pub
                    ──▶ on merge: verify again, publish catalog.json + .sig as the latest release
Laneway             ──▶ fetches releases/latest/download/catalog.json (+ .sig), checks the
                        signature with its built-in key and each release's SHA-256
```

## Releasing a Plugin

1. Bump `version` in `plugins/<plugin>/plugin.json`, with the change, and merge it.
2. Tag the merge commit `<plugin>/v<version>` and push the tag:

   ```sh
   git tag laravel/v1.2.0 && git push origin laravel/v1.2.0
   ```

The [Release workflow](../.github/workflows/release.yml) runs that Plugin's `lane plugin test` and the e2e tests against Laneway, refuses a tag that isn't the manifest's version, and publishes the tag's GitHub release with:

- `<plugin>-<version>.tar.gz`: what Laneway downloads, a reproducible gzipped tar of the manifest, the built WASM module and any Engine manifests, at its top. Source and test cases stay here.
- `plugin.json`: the manifest, to read without unpacking.
- `SHA256SUMS`: the archive's SHA-256, which the Catalog lists.

A Plugin release is never marked "latest": that is kept for the Catalog's release (below). To build a release locally: `go run ./cmd/plugins pack -tag laravel/v1.2.0 laravel` (into `dist/`).

## Signing the Catalog

After publishing a release, the Release workflow rebuilds the Catalog from every Plugin release (`scripts/build-catalog.sh`) and opens a pull request, `Catalog: <plugin> <version>`, changing `catalog/catalog.json`. Its **Catalog** check fails until the Catalog is signed. On the machine that holds the key (see [keys/README.md](../keys/README.md)), with the GitHub CLI signed in:

```sh
git fetch origin && git switch catalog/laravel-1.2.0
scripts/sign-catalog.sh /path/to/catalog.key
git add catalog/catalog.json.sig
git commit -m "chore: sign the Catalog listing laravel 1.2.0"
git push
```

`sign-catalog.sh` downloads every release, rebuilds the Catalog itself and refuses to sign unless `catalog/catalog.json` is exactly that, so a maintainer only ever signs what the releases are, whoever wrote the file. It then writes `catalog/catalog.json.sig` and checks it against `keys/catalog.pub`. With the check green, merge the pull request: the [Catalog workflow](../.github/workflows/catalog.yml) verifies the signature again and publishes `catalog.json` and `catalog.json.sig` as a new release tagged `catalog/<timestamp>`, marked latest, which is where Laneway fetches them from (`DefaultURL` in Laneway's `internal/catalog/fetch.go`).

To sign without a pull request (say, after releasing several Plugins at once): `scripts/build-catalog.sh`, review the diff, `scripts/sign-catalog.sh <key>`, and commit both files to main through a pull request as usual.

The Catalog's format is Laneway's: `internal/catalog/catalog.go` here is a copy of Laneway's, and `go run ./cmd/plugins verify` checks a Catalog exactly as Laneway does.

## Repository settings

- **Secret `LANEWAY_REPO_TOKEN`**: a fine-grained token with read access to `lanewayhq/laneway`'s contents, so CI can check out and build `lane`.
- **Variable `LANEWAY_REF`** (optional): the `lanewayhq/laneway` branch, tag or commit CI tests against. It defaults to `spec/48-plugin-system`; point it at `main` (or a release tag) once the Plugin system is merged. The CI workflow can also be run by hand against another ref.
- **Settings → Actions → General → Workflow permissions**: allow GitHub Actions to create pull requests, for the Catalog pull requests.
- The Catalog and releases are downloaded by Laneway without credentials, so this repository's releases must be public before Laneway can fetch from it.
