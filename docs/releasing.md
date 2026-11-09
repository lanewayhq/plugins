# Releasing Plugins and the Catalog

Each first-party Plugin is released on its own, by a tag. Pushing the tag does the rest, except for one step: the Catalog, the signed list Laneway fetches Plugins from, is rebuilt from every release and signed in CI only after the maintainer approves the signing job. The private key is a secret of the `catalog-signing` Environment, which only `main` and release tags may use and which waits for the maintainer's approval on every run.

```
tag laravel/v1.2.0 ──▶ Release workflow: refuse a tag that isn't on main, test against
                        Laneway, publish the Plugin's release
                         └─▶ Catalog workflow: the publish job waits for approval in `catalog-signing`
maintainer          ──▶ approve it (GitHub web or mobile app)
Catalog workflow    ──▶ rebuild the Catalog from every release, sign it, verify it with
                        keys/catalog.pub, publish catalog.json + .sig as the latest release
Laneway             ──▶ fetches releases/latest/download/catalog.json (+ .sig), checks the
                        signature with its built-in key and each release's SHA-256
```

## Releasing a Plugin

1. Bump `version` in `plugins/<plugin>/plugin.json`, with the change, and merge it.
2. Tag the merge commit `<plugin>/v<version>` and push the tag:

   ```sh
   git switch main && git pull
   git tag laravel/v1.2.0 && git push origin laravel/v1.2.0
   ```

   Push tags one at a time: GitHub starts no workflows for a push of more than three tags.
3. Approve the signing (below). That's the last step.

The [Release workflow](../.github/workflows/release.yml) refuses a tag on a commit that isn't on `main`, so only reviewed code is released. It runs that Plugin's `lane plugin test` and the e2e tests against Laneway, refuses a tag that isn't the manifest's version, and publishes the tag's GitHub release with:

- `<plugin>-<version>.tar.gz`: what Laneway downloads, a reproducible gzipped tar of the manifest, the built WASM module and any Engine manifests, at its top. Source and test cases stay here.
- `plugin.json`: the manifest, to read without unpacking.
- `SHA256SUMS`: the archive's SHA-256, which the Catalog lists.

A Plugin release is never marked "latest": that is kept for the Catalog's release (below). Until the Catalog lists it, no Laneway sees it. To build a release locally: `go run ./cmd/plugins pack -tag laravel/v1.2.0 laravel` (into `dist/`).

## Signing the Catalog

After publishing a release, the Release workflow starts the [Catalog workflow](../.github/workflows/catalog.yml), whose `publish` job waits in the `catalog-signing` Environment for your approval. GitHub notifies you. Approve it on the run's page (**Review deployments**, tick `catalog-signing`, **Approve and deploy**) or in the GitHub mobile app.

Once approved, the job checks that its commit is on `main`, rebuilds the Catalog from every Plugin release on GitHub (`scripts/build-catalog.sh`), signs it with the `CATALOG_SIGNING_KEY` secret, checks the signature against `keys/catalog.pub` as Laneway does, and publishes `catalog.json` and `catalog.json.sig` as a new release tagged `catalog/<timestamp>`, marked latest, which is where Laneway fetches them (`DefaultURL` in Laneway's `internal/catalog/fetch.go`). The release's notes list every Plugin version it holds.

The Catalog isn't committed: the latest `catalog/<timestamp>` release is the record of what Laneway sees.

- **Several releases at once:** each tag starts its own signing run, and each run lists every release published by the time it's approved. Approve one once all the releases are out; the others then find the published Catalog up to date and publish nothing, or you can reject them.
- **Rejected or failed run:** the Plugin release stays out of the Catalog until the next approved run, after the next Plugin release or run by hand on `main`: `gh workflow run catalog.yml -R lanewayhq/plugins --ref main`.

**Signing by hand** (if CI can't): on a machine with the key, `scripts/sign-catalog.sh <key-file>` rebuilds `catalog/catalog.json` from the releases, signs and verifies it, and prints the `gh release create` command that publishes it.

The Catalog's format is Laneway's: `internal/catalog/catalog.go` here is a copy of Laneway's, and `go run ./cmd/plugins verify` checks a Catalog exactly as Laneway does.

## Repository settings

- **Secret `LANEWAY_DEPLOY_KEY`** (optional): the private half of a read-only deploy key on `lanewayhq/laneway`. CI checks out and builds `lane` from it, and since the repository is public, the workflow's own token is enough without it; the key (or `LANEWAY_REPO_TOKEN`, a fine-grained token with read access to its contents) is only needed if it goes private again. Pull requests from forks get no secrets, so they always use the workflow's token.
- **Variable `LANEWAY_REF`** (optional): the `lanewayhq/laneway` branch, tag or commit CI tests against. It defaults to `main`. The CI workflow can also be run by hand against another ref.
- **Environment `catalog-signing`**: required reviewer the maintainer, deployments limited to the branch `main` and tags matching `*/v*` (the Release workflow signs from the tag it releases), holding the secret `CATALOG_SIGNING_KEY` (see [keys/README.md](../keys/README.md)). Admins can change these rules, so keep the org's and this repository's admins to the maintainer.
- **Tag ruleset `release tags`**: only repository admins may create, move or delete tags matching `*/v*`, so no one else can start a release.
- **Branch protection on `main`**: pull requests only, with the `test` check required. No approval is required, since the maintainer is the only one with write access and can't approve their own pull requests: only the maintainer can merge, so the signing workflow can't change without them. [CODEOWNERS](../.github/CODEOWNERS) still asks for their review on every pull request. Give anyone else write access only together with bringing back a required code-owner approval.
- The Catalog and releases are downloaded by Laneway without credentials, so this repository's releases must be public before Laneway can fetch from it.
