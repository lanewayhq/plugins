# Contributing to Laneway's Plugins

Thanks for helping. The full flow, for features, bug fixes, contributions and releases across this repository and [lanewayhq/laneway](https://github.com/lanewayhq/laneway), is in Laneway's [docs/development-flow.md](https://github.com/lanewayhq/laneway/blob/main/docs/development-flow.md). The short version:

1. **Start with an issue**, unless the change is small and obvious. Pull requests aren't a feature-request channel: agree the approach in an issue first. A new first-party Plugin always starts with an issue.
2. **Fork, and branch from `main`.**
3. **Test it against Laneway's `main`.** Build `lane` from [lanewayhq/laneway](https://github.com/lanewayhq/laneway) (`go build -o lane ./cmd/lane`), then:
   ```sh
   go run ./cmd/plugins build
   LANE=/path/to/lane go run ./cmd/plugins test         # lane plugin test, for every Plugin
   lane plugin test --update plugins/<name>             # accept new output; review the golden files' diff
   LANE=/path/to/lane LANEWAY_SRC=/path/to/laneway go test -tags e2e ./e2e
   gofmt -w <your files>
   ```
   Add or update the Plugin's `lane plugin test` cases under `plugins/<name>/testdata/`. A bug fix starts with a case that fails without it.
4. **Follow the conventions:** commit messages like `fix: <what is now true, as a sentence>` (`feat`, `fix`, `docs`, `test`, `refactor`, `chore`, `ci`), and the words Laneway's [GLOSSARY.md](https://github.com/lanewayhq/laneway/blob/main/GLOSSARY.md) defines.
5. **Leave releases to the maintainer:** don't bump a Plugin's `version`, and don't touch `catalog/`, `keys/` or `.github/` unless the issue asks for it.
6. **Open the pull request** against `main`, with `Fixes #N` and how you tested it. The maintainer approves the first CI run from a fork and reviews every change.

**Your own Plugin doesn't need to be here.** This repository holds only the first-party Plugins listed in the Catalog. Publish yours in your own repository, under any license, and users add it with `lane plugin add github.com/<owner>/<repo>`. See the [authoring guide](docs/authoring.md).

**Security issues:** don't open a public issue. Contact the maintainer privately.

By contributing, you agree that your contribution is licensed under the [Apache License 2.0](LICENSE).
