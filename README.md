# Laneway Plugins

First-party Plugins for [Laneway](https://github.com/lanewayhq/laneway), the Go SDK for writing Plugins, and the signed Catalog that lists them (see Laneway's [ADR-0007](https://github.com/lanewayhq/laneway/blob/main/docs/adr/0007-plugins-are-wasm-effects-from-a-signed-catalog.md)).

| Plugin | Adds |
| --- | --- |
| [laravel](plugins/laravel) | the Laravel Recipe |
| [rails](plugins/rails) | the Rails Recipe |
| [vite](plugins/vite) | the Vite Integration |
| [postgres](plugins/postgres), [mysql](plugins/mysql), [redis](plugins/redis), [mailpit](plugins/mailpit), [rustfs](plugins/rustfs) | Engines (manifests only) |
| [claude](plugins/claude), [t3](plugins/t3) | the Claude Code and T3 Code Harness Generators |
| [boost](plugins/boost) | the Laravel Boost Generator |

## Layout

- `sdk/`: the Go SDK, `github.com/lanewayhq/plugins/sdk`. [docs/authoring.md](docs/authoring.md) is the guide to writing a Plugin.
- `plugins/<name>/`: each Plugin: its `plugin.json`, its source or Engine manifests, and its `lane plugin test` cases under `testdata/`.
- `cmd/plugins/`: builds, tests and packs the Plugins, and builds, signs and verifies the Catalog.
- `keys/catalog.pub`: the public half of the key the Catalog is signed with (see [keys/README.md](keys/README.md)). The signed Catalog itself isn't committed: it is published as this repository's latest release, `catalog/<timestamp>`.
- `e2e/`: tests that drive a real Laneway build with each Plugin's release.

## Working on a Plugin

You need Go and a `lane` built from [lanewayhq/laneway](https://github.com/lanewayhq/laneway) (`go build -o lane ./cmd/lane`).

```sh
go run ./cmd/plugins build                    # build every Plugin's WASM module (plugins/*/plugin.wasm, not committed)
LANE=/path/to/lane go run ./cmd/plugins test  # run `lane plugin test` on every Plugin
lane plugin test --update plugins/laravel     # accept what a Plugin now returns; review the golden files' diff
LANE=/path/to/lane LANEWAY_SRC=/path/to/laneway go test -tags e2e ./e2e
```

[docs/releasing.md](docs/releasing.md) describes releases, the Catalog and signing, and [CONTRIBUTING.md](CONTRIBUTING.md) how to contribute.

## License

The SDK and every Plugin here are licensed under the [Apache License 2.0](LICENSE). Plugins you build with the SDK can use any license you like.
