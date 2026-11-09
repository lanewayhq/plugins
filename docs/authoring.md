# Writing a Laneway Plugin

A Plugin teaches Laneway about something it doesn't know: a stack (a **Recipe**), a tool used alongside any stack (an **Integration**), backing infrastructure (an **Engine**), a tool's committed config (a **Generator**), reactions to the dev environment's lifecycle (**hooks**), commands (`lane <plugin> <cmd>`) and links on the Dashboard.

A Plugin is a directory with a manifest, `plugin.json`, and, unless it only adds Engines, a WASM module. Laneway runs the module in a sandbox: it sends one JSON request on stdin, with the Project's directory mounted read-only, and reads one JSON response from stdout. The module never acts on its own. It returns a config layer or **effects** (env keys, commands to run, file blocks to merge, HTTP requests, warnings), and Laneway applies them under its usual rules: it only writes what it owns, merges rather than overwrites, and is safe to re-run. See [ADR-0007](https://github.com/lanewayhq/laneway/blob/main/docs/adr/0007-plugins-are-wasm-effects-from-a-signed-catalog.md) for why.

The first-party Plugins in [`plugins/`](../plugins) are worked examples. Each is written the way this guide describes.

## A first Plugin

Here is a Recipe for Projects with a `Procfile`, written in Go with the SDK:

```
procfile/
  plugin.json
  main.go
  testdata/web-and-worker/project/Procfile
  testdata/web-and-worker/golden.json      (written by `lane plugin test --update`)
```

```json
{
  "name": "procfile",
  "version": "0.1.0",
  "api_version": 1,
  "recipes": [{"name": "procfile", "detect": {"file": "Procfile"}}]
}
```

```go
package main

import (
	"strings"

	"github.com/lanewayhq/plugins/sdk"
)

func main() {
	sdk.Run(sdk.Plugin{Recipes: map[string]sdk.Recipe{"procfile": recipe}})
}

func recipe(p sdk.Project) (sdk.RecipeResult, error) {
	data, err := p.ReadFile("Procfile")
	if err != nil {
		return sdk.RecipeResult{}, err
	}
	l := sdk.Layer{Processes: map[string]sdk.Process{}}
	for line := range strings.SplitSeq(string(data), "\n") {
		name, command, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		l.Processes[name] = sdk.Process{Command: strings.TrimSpace(command), Primary: name == "web"}
	}
	return sdk.RecipeResult{Layer: l}, nil
}
```

Build the module, run its tests, and add it to your Laneway:

```sh
GOOS=wasip1 GOARCH=wasm go build -o plugin.wasm .
lane plugin test --update    # write the golden files; review them
lane plugin test             # from now on, compare against them
lane plugin add .            # shows what the Plugin may do, then asks
```

Any language that compiles to a WASI command (`wasip1`) works: Go, TinyGo, Rust, or JavaScript through a WASI runtime. The SDK only saves you the JSON plumbing; the protocol is described under [The protocol](#the-protocol).

## The manifest

`plugin.json` sits at the top of the Plugin's directory. Laneway refuses a manifest with keys it doesn't know, so a typo is an error rather than ignored.

| Key | What it is |
| --- | --- |
| `name` | The Plugin's name: lowercase letters, digits and `-`, starting with a letter. It names its directory, its `lane <name> <cmd>` commands and `lane generate <name>`. |
| `version` | The Plugin's own version, such as `"1.2.0"`. Releases are tagged with it (see [releasing](releasing.md)). |
| `api_version` | The Plugin API version it targets. Until Laneway 1.0 only the current version, `1`, is accepted, and Laneway says so clearly when it isn't. |
| `module` | Its WASM module, relative to its directory. Defaults to `plugin.wasm`. Only a Plugin with calls to answer needs one. |
| `recipes` | Recipes it adds: `{"name", "detect"}` each. |
| `integrations` | Integrations it adds: `{"name", "detect"}` each. |
| `engines` | Engine manifests (TOML files) it adds, relative to its directory. |
| `generators` | Generators it adds: `{"name"}` each, run in order by `lane generate <plugin>`. |
| `commands` | Commands it adds: `{"name", "summary"}` each, run as `lane <plugin> <name> [args...]`. |
| `hooks` | The lifecycle events it hooks, such as `["ready"]`. |
| `dashboard` | Links, badges and actions it shows on the Dashboard. |
| `capabilities` | What it may do beyond returning config: `effects`, the effect kinds it may return, and `http`, the hosts its http effects may reach. |

A Plugin must add something. Laneway shows the developer everything the manifest declares, including its capabilities, before it's added, and refuses any effect whose kind the manifest doesn't declare.

```json
{
  "name": "notify",
  "version": "1.0.0",
  "api_version": 1,
  "hooks": ["ready"],
  "capabilities": {"effects": ["http"], "http": ["ntfy.sh"]}
}
```

## Detection rules

A Recipe or Integration says which Projects it recognises with a detection rule: data, not code, so that Laneway can match a Project against a Catalog Plugin before downloading it. A rule has exactly one key:

| Rule | Matches when |
| --- | --- |
| `{"file": "artisan"}` | the file exists, relative to the Project's directory |
| `{"composer": "laravel/boost"}` | `composer.json` requires the package, in `require` or `require-dev` |
| `{"gem": "rails"}` | the `Gemfile` declares the gem |
| `{"package": "vite"}` | `package.json` depends on the package, in `dependencies` or `devDependencies` |
| `{"all": [rule, ...]}` | every rule matches |
| `{"any": [rule, ...]}` | one of the rules matches |

The rails Recipe, for example, needs a Rails app's `config/application.rb` and the `rails` or `railties` gem:

```json
{"all": [{"file": "config/application.rb"}, {"any": [{"gem": "rails"}, {"gem": "railties"}]}]}
```

`lane info` shows each Project's Recipe and Integrations and the rule that matched, such as `artisan exists`. `lane plugin test` records the same in each golden file.

## Recipes and Integrations

A Project has at most one Recipe, its stack, and any number of Integrations, tools it uses alongside it. Both return a **config layer**, keyed like `laneway.toml` (`sdk.Layer`): Processes, `env`, `linked_env`, `services`, `[worktree]` and `[database]`. Laneway applies the Recipe's layer, then each Integration's in a stable order, then the Project's `laneway.toml` and `laneway.local.toml`, so the Project's own config always wins.

- Only a Recipe may choose the Primary Process (`primary: true`), the one served at the Worktree's Domain.
- Two Integrations that set the same key to different values are an error naming both Plugins, until the Project's config settles it.
- Values may use Laneway's placeholders, such as `{port}`, `{url}`, `{db}` or a Service's `{redis_host}`, which Laneway fills in per Worktree.
- A Recipe also says whether the Project's tests use a Test Database (`TestDatabase`, with `TestDatabaseWarnings` naming files that bypass it) and which env file its tests read (`TestEnv`).
- Warnings (`Warnings`) tell the developer about problems that don't stop the Project running. Name the file at fault.

Read the Project with `sdk.Project`: `ReadFile` and `Exists` for its files, `EnvFile()` for its `.env` (or `.env.example` before there is one), and `Gems()`, `Packages()` and `ComposerPackages()` for its dependencies. The directory is read-only. A Worktree other than the main one also sees the main Worktree, read-only, at `MainDir`, for files it copies from there.

## Engines

An Engine is data: a TOML manifest listed under `engines`, with no module. It declares the container image, ports, data directory, user and environment, a readiness probe, and the commands that manage its databases, run inside the Service's container. See [`plugins/postgres/postgres.toml`](../plugins/postgres/postgres.toml) for a full one, and [`plugins/redis/redis.toml`](../plugins/redis/redis.toml) for the smallest.

```toml
name = "redis"            # lowercase letters, digits and _; config names it, and its Service variables start with it
default_version = "7"     # the version that gets port; others get a derived port
image = "docker.io/library/redis"   # fully qualified; a Service at version 7 runs redis:7 (or see [versions] below)
port = 6379               # its standard port, inside and outside the container
data_dir = "/data"        # a named volume is mounted here
command = ["redis-server", "--requirepass", "{password}", "--appendonly", "yes"]

[[ready]]
kind = "redis-ping"
```

Readiness probes are tried in order until each passes. Their `kind` is one of:

| Kind | Passes when |
| --- | --- |
| `banner` | the first line on a new connection starts with `expect`, such as SMTP's `"220"` |
| `redis-ping` | the Engine answers a Redis `PING` |
| `http` | `GET path` (default `/`) on `port` answers `status` (default 200) |
| `command` | `command`, run in the container, exits 0 |

`[database]` makes the Engine able to hold a Project's database: `max_name`, and `exists`, `create`, `clone`, `drop` and `list`, each a command run in the container without a shell (use `["sh", "-c", ...]` for more). Placeholders: `{user}`, `{password}` and `{version}` in what configures the container; `{db}`, `{from}` and `{to}` in the database commands; `{{` and `}}` for literal braces. A command fails by exiting non-zero and saying why on stderr.

Engine names must be unique across the Plugins a developer has. A Dashboard link scoped to `service` shows on each Service of the Plugin's Engines, such as Mailpit's Inbox at `https://mail.{dashboard_domain}/`.

### Versions mapped to images

By default a Service's version is its image's tag: `redis = "7"` runs `docker.io/library/redis:7`. An image with no tag per major version, such as RustFS's (only `1.0.4`), or one you want pinned by digest, maps each version to an image reference in `[versions]` instead of setting `image`:

```toml
name = "rustfs"
default_version = "1"

[versions."1"]
image = "docker.io/rustfs/rustfs:1.0.4"     # or "docker.io/rustfs/rustfs@sha256:…"
```

- Each `image` is fully qualified, with a tag or a `sha256` digest. `image` and `[versions]` can't both be set, and `image` is the only key in a version's table.
- `default_version` must be one of the versions. A Project can only declare a version the Engine maps: any other is a config error naming the Engine and its versions.
- A Service, its named volume and its port belong to its version, never its image. So releasing your Plugin with a version mapped to a new image, such as `1.0.5`, moves every Service at that version to it on `lane plugin update`: its container is stopped and created again from the new image, with the data in its volume and at the same port. Only map a version to images that can read each other's data, as patch releases normally can; give a release that can't a new version instead.

### Service Domains

An Engine that serves browsers, such as Mailpit's inbox, declares **Service Domains**: Domains under the Dashboard's at which Laneway serves one of its container ports over HTTPS. Each is a `label` and the container `port` it forwards to, which must be `port` or `web_port` (a second port the container publishes, such as a web UI's). Mailpit's inbox is at `https://mail.laneway.localhost`:

```toml
port = 1025               # SMTP
web_port = 8025           # the inbox

[[domains]]
label = "mail"            # served at mail.<Dashboard Domain>
port = 8025
```

A Service Domain serves the Service of the Engine's default version if a Project uses it, otherwise the first one that is used. The first request starts the Service, and every request (a WebSocket for as long as it is open) keeps it from going Idle. When no Project uses the Engine, the Domain says what to declare.

- A label is one lowercase Domain label (letters, digits and `-`), unique across the Engines a developer has: two Plugins declaring the same one can't both be added.
- No Project or Worktree can be served at a Service Domain: they are under the Dashboard's Domain.
- The first Service Domain's URL, such as `https://mail.laneway.localhost` (no trailing slash), is the Service variable `{<engine>_endpoint}`, which env templates may use like `{redis_host}`.

### Buckets

An object storage Engine, such as RustFS, declares `[bucket]`: the commands that give each Worktree its own **Bucket** in the Service. A Project that uses the Engine then gets a private Bucket in each Worktree, made while the Worktree is Preparing, and its name is the Service variable `{<engine>_bucket}`. The main Worktree's starts empty; any other Worktree's starts as a copy of the main Worktree's, unless that holds more than `bucket_copy_limit` (1 GB by default), when it starts empty with a warning.

```toml
[bucket]
exists = ["sh", "-c", '''…''', "sh", "{bucket}"]
create = ["sh", "-c", '''…''', "sh", "{bucket}"]
clone = ["sh", "-c", '''…''', "sh", "{from}", "{to}"]
configure = ["sh", "-c", '''…''', "sh", "{bucket}", "{public}", "{origins}"]
drop = ["sh", "-c", '''…''', "sh", "{bucket}"]
list = ["sh", "-c", '''…''']
size = ["sh", "-c", '''…''', "sh", "{bucket}"]
```

The commands follow the `[database]` contract: each runs in the container without a shell, and fails by exiting non-zero and saying why on stderr. `{bucket}`, `{from}` and `{to}` are Bucket names Laneway made: lowercase letters, digits and `-`, 3 to 63 characters, safe in a URL or a shell script as they are. Pass them as arguments after the script, as above, rather than writing them into the script. Credentials are best read from the container's environment, so they never appear in a command's arguments.

| Command | Must |
| --- | --- |
| `exists` | print the name if `{bucket}` exists and nothing if it doesn't; exit non-zero for any other answer, such as a refused login. A copy `clone` left unfinished doesn't exist |
| `create` | create `{bucket}`, empty and private; it only runs once `exists` said no |
| `clone` | create `{to}` as a copy of every object in `{from}`, with its metadata; it only runs once `exists` said `{to}` doesn't exist, and must leave `{to}` complete or absent: if it fails, or never finishes, `exists` must not see `{to}` |
| `configure` | set who may reach `{bucket}`: if `{public}` is `true`, anyone may read its objects without signing (a bucket policy allowing anonymous `s3:GetObject`); if `false`, no one may; and browsers may make requests to it (CORS) from exactly `{origins}`, or from nowhere if it is empty. Each run replaces what the last one set |
| `drop` | drop `{bucket}` with everything in it, its policy and CORS too, and succeed when it doesn't exist |
| `list` | print every Bucket in the Service, one per line |
| `size` | print the total size of `{bucket}`'s objects in bytes, a decimal integer on a line of its own |

`configure` runs after `create` or `clone` (so `clone` needn't copy a policy or CORS), every time the Worktree is prepared, whether its Bucket was made then or not, and again whenever the Worktree gains or loses an Alias. `{origins}` is one argument: the Worktree's Domain and its Aliases' as `https://` origins separated by spaces, such as `https://feature.shop.localhost https://checkout.shop.localhost`. Loop over it in the script (`for o in $origins`) rather than expecting one argument each. `{public}` is `true` or `false`; exit with status 2 on anything else. Public versus private is decided per Bucket, never per object: don't honour object ACLs.

A Bucket is named after its Project and Worktree: `shop` for the main Worktree and `shop--feature-x` for the Worktree `feature-x`, cut short with a hash when too long. Every key of `[bucket]` is required, and an unknown one is an error. Write `{{` for a literal brace, such as curl's `-w '%{{http_code}}'`.

#### Buckets a Project declares

A Project may declare more Buckets than its private one, such as a public one for a production disk anyone can read from, in a `[buckets]` table in `laneway.toml` or `laneway.local.toml`:

```toml
[buckets.public]
public = true             # anyone may read its objects; false (the default) for private

[buckets.exports]         # private

[env]
PUBLIC_BUCKET = "{rustfs_bucket_public}"
```

Each Worktree then gets each declared Bucket in each object storage Service its Project uses, made, copied and configured like the private one: `shop-public` in the main Worktree and `shop-public--feature-x` in `feature-x`. Its name is the variable `{<engine>_bucket_<name>}`. A name is lowercase letters, digits and `_`, starting with a letter (`user_uploads` gives `shop-user-uploads`), and `public` is a Bucket's only key: anything else is an error. A removed Worktree's declared Buckets become Orphans like its private one. Your Engine's commands needn't know which Bucket is which: Laneway passes `{public}` to `configure`.

## Hooks and events

A Plugin hooks lifecycle events by listing them under `hooks`. Its module gets a `hook` call for each, with the event (`sdk.Event`: the Project, and the Worktree, Process or Service it happened to), and returns effects.

| Event | When |
| --- | --- |
| `link`, `unlink` | a Project was Linked or Unlinked |
| `preparing` | a Worktree is being prepared; runs as a setup step, after `on_create` |
| `ready` | a Worktree became Ready |
| `worktree_removed` | a Worktree was removed (the main Worktree is mounted) |
| `primary_domain_changed` | a Worktree's Primary Domain moved after an Alias was added or removed |
| `process_started`, `process_stopped`, `process_crashed` | a Worktree's Process started, was stopped, or exited on its own |
| `idle`, `woken` | a Worktree went Idle, or a request woke it |
| `service_started`, `service_stopped` | a Service the Project uses started or stopped |

Only `preparing` can block: its error, or a failed exec effect, fails setup and keeps the Worktree from becoming Ready, recorded with its output like a failed setup step. Every other hook runs in the background, and its failures become warnings. A Plugin's hooks fire only for Projects that use it: its Recipe or an Integration was detected, its Engine is used, or the Project's config names it under `plugins`. A **machine-wide** Plugin, added with `lane plugin add --global`, is turned on only by personal config (the global config or `laneway.local.toml`), never by a committed `laneway.toml`.

## Effects

Hooks, commands and Generators return effects, in order. A Plugin may only return the kinds its manifest lists under `capabilities.effects`.

| Kind | SDK | What Laneway does |
| --- | --- | --- |
| `env` | `sdk.Env(map)` | sets keys in the Worktree's `.env`; keys Laneway's Env Patch sets are left to it, with a warning |
| `exec` | `sdk.Exec("php", "artisan", "migrate")` | runs the command in the Worktree with its Runtimes, without a shell |
| `file` | `sdk.File(block)` | merges a block into a file in the Worktree (see below) |
| `http` | `sdk.HTTP(req)` | sends the request, only to hosts under `capabilities.http`; a non-2xx response fails the effect |
| `warning` | `sdk.Warning(msg)` | shows the developer a warning |

A file block (`sdk.FileBlock`) is the part of one file Laneway owns:

- `sdk.Markdown(path, body)`: the body of Laneway's marked block, created if missing. `RemoveMarkdown` takes it out again.
- `JSON`: what Laneway owns in a JSON object, merged structurally. Each entry sets a key, or, with `Match`, owns the array element whose keys match. `New` is what a file that doesn't exist yet starts as.

Laneway never overwrites the rest of the file, and merging the same block again changes nothing.

## Generators

A Generator merges a tool's config into the developer's committed files when they run `lane generate <plugin>`, and returns only file and warning effects. Generated files are committed, so what a Generator writes must be harmless for teammates without Laneway. The claude and t3 Generators, for example, add a hook that waits until the Worktree is Ready and succeeds quietly when `lane` isn't installed. Laneway merges every block at once, or none if one can't be merged, and tells the developer when generated files are out of date after a Plugin update, without rewriting them itself.

## Commands and the Dashboard

A command, `lane <plugin> <name> [args...]`, runs in the Worktree it was typed in and returns effects, such as `sdk.Exec` running a dump tool. Its handler gets the arguments after its name.

The Dashboard shows what the manifest declares, as data. A Plugin supplies no UI of its own:

```json
"dashboard": {
  "links":   [{"scope": "service", "label": "Inbox", "url": "https://mail.{dashboard_domain}/"}],
  "badges":  [{"scope": "worktree", "text": "seeded", "title": "Seeded by db:seed"}],
  "actions": [{"scope": "worktree", "label": "Dump", "command": "dump"}]
}
```

Each item has a scope, which decides where it shows and which placeholders it may use: `project` (`{dashboard_domain}`, `{project}`, `{domain}`), `worktree` (adds `{worktree}`) and `service` (`{dashboard_domain}`, `{service}`, `{engine}`, `{version}`, `{host}`, `{port}`). An action runs one of the Plugin's commands, so it can't be scoped to a Service.

## Testing with `lane plugin test`

`lane plugin test [dir]` runs the Plugin, as built in its directory, against its test cases, with no daemon. Each case is a directory under `testdata/`:

```
testdata/<case>/project/        a fixture Project, mounted as the Project
testdata/<case>/events/*.json   sample events for its hooks
testdata/<case>/golden.json     what the Plugin returned, as accepted
```

For a fixture Project, the Recipes and Integrations whose rules detect it return their layers (with the rule that matched), and the Generators their effects. For each sample event, the hook returns its effects. The golden file records all of it, along with the warnings Laneway would show and the effects it would refuse. When what the Plugin returns differs from the golden file, `lane plugin test` shows the diff and fails. `lane plugin test --update` rewrites the golden files: review the diff, then commit it with the change that caused it. A pull request to a Plugin therefore shows, in its golden files, what the Plugin now does.

A sample event is an `sdk.Event` as JSON:

```json
{
  "name": "ready",
  "project": {"name": "shop", "path": "/home/dev/shop"},
  "worktree": {"name": "checkout", "path": "/home/dev/shop-checkout", "domain": "checkout.shop.localhost", "url": "https://checkout.shop.localhost"}
}
```

Cover each way the Plugin's answer can go: Projects its rules shouldn't detect (their golden file is `{}`), each configuration it reads, and the files it warns about. Go tooling ignores `testdata`, so a Go Plugin keeps its cases beside its source.

In this repository, `go run ./cmd/plugins test` builds every Plugin and runs `lane plugin test` on each; set `LANE` to the `lane` binary to use. CI also runs the e2e tests in [`e2e/`](../e2e) against a Laneway build.

## The protocol

The SDK is optional. A module is a WASI command (`wasip1`) that reads one JSON request on stdin and writes one JSON response on stdout; what it writes to stderr goes to Laneway's logs. Calls have a time limit, and there are no host functions and no sockets.

```json
{"api_version": 1, "call": "recipe", "name": "procfile", "dir": "/project", "host_dir": "/home/dev/shop"}
```

`call` is `recipe`, `integration`, `generator`, `command` (with `args`) or `hook` (with `event`), and `name` is the Recipe, Integration, Generator, command or event it is for. `dir` is the mounted Project, also the working directory; `host_dir` is where it is on the developer's machine, for reading absolute paths Laneway wrote into its files; `main_dir` is the main Worktree when `dir` is another one.

The response is `{"error": "..."}`, said for the developer, or the result: `{"layer": {...}, "warnings": [...]}` for a Recipe or Integration (a Recipe adds `test_database`, `test_database_warnings` and `test_env`), and `{"effects": [...]}` for the rest. The SDK's types (`sdk.RecipeResult`, `sdk.Effect` and so on) are the reference.

## Publishing

A Plugin you write is added by hand from its directory (`lane plugin add ./my-plugin`), and from GitHub (`lane plugin add github.com/you/my-plugin`) once Laneway supports it, and is pinned to the hash it had when added. Only the first-party Plugins in this repository are in the Catalog, which Laneway fetches from on its own. [releasing.md](releasing.md) describes how they are released.
