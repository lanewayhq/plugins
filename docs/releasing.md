# Releasing Plugins and the Catalog

Each first-party Plugin is released on its own, by a tag. The Catalog, the signed list Laneway fetches Plugins from, is rebuilt from every release and signed in CI, but only after the maintainer approves the signing job: the private key is a secret of the `catalog-signing` Environment, which only runs on `main` and waits for the maintainer's approval on every run.

```
tag laravel/v1.2.0 ──▶ Release workflow: test against Laneway, publish the release
                         └─▶ rebuild the Catalog, open a "Catalog: laravel 1.2.0" PR
maintainer          ──▶ review the PR (the Catalog check proves it's what the releases build to), merge
Catalog workflow    ──▶ on main: the publish job waits for approval in `catalog-signing`
maintainer          ──▶ approve it (GitHub web or mobile app)
Catalog workflow    ──▶ rebuild and compare again, sign, verify with keys/catalog.pub,
                        publish catalog.json + .sig as the latest release
Laneway             ──▶ fetches releases/latest/download/catalog.json (+ .sig), checks the
                        signature with its built-in key and each release's SHA-256
```

## Releasing a Plugin

1. Bump `version` in `plugins/<plugin>/plugin.json`, with the change, and merge it.
2. Tag the merge commit `<plugin>/v<version>` and push the tag:

   ```sh
   git tag laravel/v1.2.0 && git push origin laravel/v1.2.0
   ```

   Push tags one at a time: GitHub starts no workflows for a push of more than three tags. Releasing several Plugins at once opens several Catalog pull requests; each lists every release before it, so merge the newest and close the rest.

The [Release workflow](../.github/workflows/release.yml) runs that Plugin's `lane plugin test` and the e2e tests against Laneway, refuses a tag that isn't the manifest's version, and publishes the tag's GitHub release with:

- `<plugin>-<version>.tar.gz`: what Laneway downloads, a reproducible gzipped tar of the manifest, the built WASM module and any Engine manifests, at its top. Source and test cases stay here.
- `plugin.json`: the manifest, to read without unpacking.
- `SHA256SUMS`: the archive's SHA-256, which the Catalog lists.

A Plugin release is never marked "latest": that is kept for the Catalog's release (below). To build a release locally: `go run ./cmd/plugins pack -tag laravel/v1.2.0 laravel` (into `dist/`).

## Signing the Catalog

After publishing a release, the Release workflow rebuilds the Catalog from every Plugin release (`scripts/build-catalog.sh`) and opens a pull request, `Catalog: <plugin> <version>`, changing `catalog/catalog.json`. Its **Catalog** check rebuilds the Catalog from the releases itself and fails unless the file is exactly that, so the diff you review is what will be signed.

Merge it. The [Catalog workflow](../.github/workflows/catalog.yml)'s `publish` job then waits in the `catalog-signing` Environment for your approval (GitHub notifies you; approve on the run's page or in the mobile app). Once approved it rebuilds and compares the Catalog again, signs it with the `CATALOG_SIGNING_KEY` secret, checks the signature against `keys/catalog.pub` as Laneway does, and publishes `catalog.json` and `catalog.json.sig` as a new release tagged `catalog/<timestamp>`, marked latest, which is where Laneway fetches them (`DefaultURL` in Laneway's `internal/catalog/fetch.go`).

Several releases can share one signing: merge their Catalog pull requests (each later one lists every earlier release) and approve only the last run.

**Signing by hand** (if CI can't): on a machine with the key, `scripts/build-catalog.sh`, review the diff, `scripts/sign-catalog.sh <key-file>`, then publish the two files with `gh release create catalog/$(date -u +%Y%m%d%H%M%S) catalog/catalog.json catalog/catalog.json.sig --latest`.

The Catalog's format is Laneway's: `internal/catalog/catalog.go` here is a copy of Laneway's, and `go run ./cmd/plugins verify` checks a Catalog exactly as Laneway does.

## Repository settings

- **Secret `LANEWAY_DEPLOY_KEY`** (optional): the private half of a read-only deploy key on `lanewayhq/laneway`. CI checks out and builds `lane` from it, and since the repository is public, the workflow's own token is enough without it; the key (or `LANEWAY_REPO_TOKEN`, a fine-grained token with read access to its contents) is only needed if it goes private again. Pull requests from forks get no secrets, so they always use the workflow's token.
- **Variable `LANEWAY_REF`** (optional): the `lanewayhq/laneway` branch, tag or commit CI tests against. It defaults to `main`. The CI workflow can also be run by hand against another ref.
- **Settings → Actions → General → Workflow permissions**: allow GitHub Actions to create pull requests, for the Catalog pull requests.
- **Environment `catalog-signing`**: required reviewer the maintainer, deployment branches limited to `main`, holding the secret `CATALOG_SIGNING_KEY` (see [keys/README.md](../keys/README.md)). Admins can change these rules, so keep the org's and this repository's admins to the maintainer.
- **Branch protection on `main`**: pull requests only, with the `test` check required. No approval is required, since the maintainer is the only one with write access and can't approve their own pull requests: only the maintainer can merge, so the signing workflow can't change without them. [CODEOWNERS](../.github/CODEOWNERS) still asks for their review on every pull request. Give anyone else write access only together with bringing back a required code-owner approval.
- The Catalog and releases are downloaded by Laneway without credentials, so this repository's releases must be public before Laneway can fetch from it.
