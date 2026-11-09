//go:build e2e

// Package e2e tests the first-party Plugins' releases against a real
// Laneway build, driven only through its `lane` binary ($LANE), with a
// daemon of its own in a throwaway home:
//
//	go run ./cmd/plugins build
//	LANE=/path/to/lane LANEWAY_SRC=/path/to/laneway go test -tags e2e ./e2e
//
// LANEWAY_SRC, Laneway's source, is optional: with it, the Plugins are also
// checked against the copies Laneway still builds in, for as long as it
// does.
package e2e

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/lanewayhq/plugins/internal/catalog"
)

// root is the repository's root, from this package's directory.
const root = ".."

var lane string

func TestMain(m *testing.M) {
	os.Exit(run(m))
}

func run(m *testing.M) int {
	lane = os.Getenv("LANE")
	if lane == "" {
		fmt.Fprintln(os.Stderr, "e2e: set LANE to the lane binary to test against")
		return 1
	}
	// A throwaway home. The daemon's socket is under XDG_RUNTIME_DIR,
	// which must be short: a Unix socket's path has a small limit.
	home, err := os.MkdirTemp("", "lw")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer os.RemoveAll(home)
	for k, v := range map[string]string{
		"HOME":            home,
		"XDG_CONFIG_HOME": filepath.Join(home, "c"),
		"XDG_STATE_HOME":  filepath.Join(home, "s"),
		"XDG_CACHE_HOME":  filepath.Join(home, "k"),
		"XDG_DATA_HOME":   filepath.Join(home, "d"),
		"XDG_RUNTIME_DIR": filepath.Join(home, "r"),
	} {
		os.Setenv(k, v)
	}
	os.MkdirAll(filepath.Join(home, "r"), 0o700)

	build := exec.Command("go", "run", "./cmd/plugins", "build")
	build.Dir = root
	if out, err := build.CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "e2e: building the Plugins: %v\n%s", err, out)
		return 1
	}
	stop, err := startDaemon(home)
	if err != nil {
		fmt.Fprintln(os.Stderr, "e2e:", err)
		return 1
	}
	defer stop()
	return m.Run()
}

// startDaemon starts `lane daemon` on free loopback ports and waits until
// the CLI reaches it.
func startDaemon(home string) (stop func(), err error) {
	https, http := freePort(), freePort()
	log, err := os.Create(filepath.Join(home, "daemon.log"))
	if err != nil {
		return nil, err
	}
	cmd := exec.Command(lane, "daemon", "--https-addr", https, "--http-addr", http)
	cmd.Stdout, cmd.Stderr = log, log
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	stop = func() {
		cmd.Process.Signal(syscall.SIGTERM)
		cmd.Wait()
		log.Close()
	}
	for deadline := time.Now().Add(30 * time.Second); time.Now().Before(deadline); time.Sleep(100 * time.Millisecond) {
		if exec.Command(lane, "plugin", "list").Run() == nil {
			return stop, nil
		}
	}
	stop()
	out, _ := os.ReadFile(log.Name())
	return nil, fmt.Errorf("the daemon didn't start:\n%s", out)
}

func freePort() string {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		panic(err)
	}
	defer l.Close()
	return l.Addr().String()
}

type result struct {
	stdout, stderr string
	err            error
}

func (r result) String() string { return r.stdout + r.stderr }

// runLane runs lane with args in dir.
func runLane(dir string, args ...string) result {
	cmd := exec.Command(lane, args...)
	cmd.Dir = dir
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	return result{stdout.String(), stderr.String(), err}
}

func mustLane(t *testing.T, dir string, args ...string) result {
	t.Helper()
	r := runLane(dir, args...)
	if r.err != nil {
		t.Fatalf("lane %s: %v\n%s", strings.Join(args, " "), r.err, r)
	}
	return r
}

// plugins are the names of the Plugins in the repository.
func plugins(t *testing.T) []string {
	t.Helper()
	manifests, err := filepath.Glob(filepath.Join(root, "plugins", "*", catalog.ManifestFile))
	if err != nil || len(manifests) == 0 {
		t.Fatalf("no Plugins: %v", err)
	}
	var names []string
	for _, m := range manifests {
		names = append(names, filepath.Base(filepath.Dir(m)))
	}
	return names
}

// release is the Plugin name's release, unpacked the way Laneway unpacks
// one it downloads, into a new directory.
func release(t *testing.T, name string) string {
	t.Helper()
	_, archive, err := catalog.PackDir(filepath.Join(root, "plugins", name))
	if err != nil {
		t.Fatal(err)
	}
	files, err := catalog.ReadArchive(archive)
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(t.TempDir(), name)
	for f, data := range files {
		path := filepath.Join(dir, filepath.FromSlash(f))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// withTestCases links the Plugin name's test cases into dir, a release of
// it, which doesn't ship them.
func withTestCases(t *testing.T, name, dir string) string {
	t.Helper()
	cases, err := filepath.Abs(filepath.Join(root, "plugins", name, "testdata"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(cases, filepath.Join(dir, "testdata")); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestEachReleasePassesItsPluginsTestCases(t *testing.T) {
	for _, name := range plugins(t) {
		t.Run(name, func(t *testing.T) {
			dir := withTestCases(t, name, release(t, name))
			res := runLane(dir, "plugin", "test")
			if res.err != nil {
				t.Fatalf("lane plugin test on the %s release: %v\n%s", name, res.err, res)
			}
		})
	}
}

// builtInRefusal is how `lane plugin add` refuses a Plugin named like one
// built into Laneway (until lanewayhq/laneway#72 takes them out).
var builtInRefusal = regexp.MustCompile(`built-in Plugin named|name of a Plugin built into Laneway`)

// add adds the release of the Plugin name in dir. While Laneway still
// builds in a Plugin of that name, it adds a copy renamed e2e-<name>, its
// Recipes, Integrations and Engines renamed likewise (its Generators keep
// their names: its module answers to them), and reports it as builtIn. It
// returns the name added, and removes it when the test ends.
func add(t *testing.T, name, dir string) (added string, builtIn bool) {
	t.Helper()
	res := runLane(dir, "plugin", "add", "--yes", dir)
	if res.err != nil && builtInRefusal.MatchString(res.stderr) {
		builtIn = true
		name = "e2e-" + name
		rename(t, dir, name)
		res = runLane(dir, "plugin", "add", "--yes", dir)
	}
	if res.err != nil {
		t.Fatalf("lane plugin add %s: %v\n%s", name, res.err, res)
	}
	if !strings.Contains(res.stdout, "Added Plugin "+name+" ") {
		t.Fatalf("lane plugin add %s said:\n%s", name, res)
	}
	t.Cleanup(func() {
		if r := runLane(dir, "plugin", "remove", name); r.err != nil {
			t.Errorf("lane plugin remove %s: %v\n%s", name, r.err, r)
		}
	})
	return name, builtIn
}

var engineName = regexp.MustCompile(`(?m)^name = "([a-z0-9-]+)"`)

// rename renames the Plugin in dir, and its Recipes, Integrations and
// Engines, to name and e2e-<their name> (e2e_<name> for an Engine).
func rename(t *testing.T, dir, name string) {
	t.Helper()
	file := filepath.Join(dir, catalog.ManifestFile)
	data, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatal(err)
	}
	m["name"] = name
	for _, key := range []string{"recipes", "integrations"} {
		decls, _ := m[key].([]any)
		for _, d := range decls {
			d := d.(map[string]any)
			d["name"] = "e2e-" + d["name"].(string)
		}
	}
	engines, _ := m["engines"].([]any)
	for _, e := range engines {
		path := filepath.Join(dir, filepath.FromSlash(e.(string)))
		toml, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		toml = engineName.ReplaceAll(toml, []byte(`name = "e2e_$1"`))
		if err := os.WriteFile(path, toml, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	data, err = json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestLanePluginAddAcceptsEachRelease(t *testing.T) {
	var added []string
	for _, name := range plugins(t) {
		n, _ := add(t, name, release(t, name))
		added = append(added, n)
	}
	list := mustLane(t, root, "plugin", "list")
	for _, n := range added {
		if !strings.Contains(list.stdout, n) {
			t.Errorf("lane plugin list doesn't show %s:\n%s", n, list)
		}
	}
}

// generators are the Plugins that add Generators.
var generators = []string{"claude", "t3", "boost"}

// TestEachGeneratorWritesWhatLanewaysBuiltInOneDoes runs each Generator
// Plugin's release with `lane generate` on each of its fixture Projects, in
// a git repository, and, while Laneway builds in a Generator of the same
// name, checks that both write the same files.
func TestEachGeneratorWritesWhatLanewaysBuiltInOneDoes(t *testing.T) {
	for _, name := range generators {
		t.Run(name, func(t *testing.T) {
			added, builtIn := add(t, name, release(t, name))
			cases, err := filepath.Glob(filepath.Join(root, "plugins", name, "testdata", "*", "project"))
			if err != nil || len(cases) == 0 {
				t.Fatalf("no fixture Projects for %s: %v", name, err)
			}
			for _, project := range cases {
				t.Run(filepath.Base(filepath.Dir(project)), func(t *testing.T) {
					got := repository(t, project)
					mustLane(t, got, "generate", added)
					if !builtIn {
						return
					}
					want := repository(t, project)
					mustLane(t, want, "generate", name)
					if g, w := tree(t, got), tree(t, want); !slices.Equal(g, w) {
						t.Errorf("lane generate %s wrote:\n%s\nbut Laneway's built-in %s writes:\n%s",
							added, strings.Join(g, "\n"), name, strings.Join(w, "\n"))
					}
				})
			}
		})
	}
}

// repository copies the fixture Project in dir into a new git repository
// with one commit.
func repository(t *testing.T, dir string) string {
	t.Helper()
	repo := t.TempDir()
	if err := copyDir(dir, repo); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"init", "-q", "-b", "main"},
		{"add", "-A"},
		{"-c", "user.name=e2e", "-c", "user.email=e2e@example.com", "commit", "-q", "-m", "fixture"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = repo
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
		}
	}
	return repo
}

// copyDir copies the directory src into dst, keeping symbolic links, which
// os.CopyFS doesn't copy.
func copyDir(src, dst string) error {
	return filepath.WalkDir(src, func(p string, e fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, p)
		target := filepath.Join(dst, rel)
		switch {
		case e.IsDir():
			return os.MkdirAll(target, 0o755)
		case e.Type()&fs.ModeSymlink != 0:
			link, err := os.Readlink(p)
			if err != nil {
				return err
			}
			os.Remove(target)
			return os.Symlink(link, target)
		default:
			data, err := os.ReadFile(p)
			if err != nil {
				return err
			}
			return os.WriteFile(target, data, 0o644)
		}
	})
}

// tree lists the files in a repository outside .git, each with its
// content (or a symbolic link's target).
func tree(t *testing.T, dir string) []string {
	t.Helper()
	var files []string
	err := filepath.WalkDir(dir, func(p string, e fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(dir, p)
		if e.IsDir() {
			if rel == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		if e.Type()&fs.ModeSymlink != 0 {
			link, err := os.Readlink(p)
			files = append(files, fmt.Sprintf("%s -> %s", rel, link))
			return err
		}
		data, err := os.ReadFile(p)
		files = append(files, fmt.Sprintf("%s:\n%s", rel, data))
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	slices.Sort(files)
	return files
}

// TestLanewaysBuiltInPluginsAgreeWithThePlugins checks, while Laneway
// still builds the first-party Plugins in, that its copies are the
// Plugins here: its WASM modules pass these Plugins' test cases, and its
// Engine manifests are these. It needs LANEWAY_SRC.
func TestLanewaysBuiltInPluginsAgreeWithThePlugins(t *testing.T) {
	src := os.Getenv("LANEWAY_SRC")
	if src == "" {
		t.Skip("LANEWAY_SRC isn't set")
	}
	checked := 0
	for _, name := range plugins(t) {
		t.Run(name, func(t *testing.T) {
			if gz := filepath.Join(src, "internal", "builtin", name, "plugin.wasm.gz"); exists(gz) {
				checked++
				dir := filepath.Join(t.TempDir(), name)
				os.MkdirAll(dir, 0o755)
				copyFile(t, filepath.Join(src, "internal", "builtin", name, "plugin.json"), filepath.Join(dir, "plugin.json"))
				gunzip(t, gz, filepath.Join(dir, "plugin.wasm"))
				if res := runLane(withTestCases(t, name, dir), "plugin", "test"); res.err != nil {
					t.Errorf("Laneway's built-in %s module doesn't pass the Plugin's test cases: %v\n%s", name, res.err, res)
				}
			}
			engines, _ := filepath.Glob(filepath.Join(src, "internal", "plugin", "builtin", name, "*.toml"))
			for _, builtIn := range engines {
				checked++
				ours, err := os.ReadFile(filepath.Join(root, "plugins", name, filepath.Base(builtIn)))
				if err != nil {
					t.Fatal(err)
				}
				theirs, err := os.ReadFile(builtIn)
				if err != nil {
					t.Fatal(err)
				}
				if !bytes.Equal(ours, theirs) {
					t.Errorf("plugins/%s/%s isn't Laneway's built-in %s", name, filepath.Base(builtIn), builtIn)
				}
			}
		})
	}
	if checked == 0 {
		t.Skip("Laneway builds in none of the Plugins any more")
	}
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func copyFile(t *testing.T, from, to string) {
	t.Helper()
	data, err := os.ReadFile(from)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(to, data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func gunzip(t *testing.T, from, to string) {
	t.Helper()
	data, err := os.ReadFile(from)
	if err != nil {
		t.Fatal(err)
	}
	r, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("%s: %v", from, err)
	}
	module, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("%s: %v", from, err)
	}
	if err := os.WriteFile(to, module, 0o644); err != nil {
		t.Fatal(err)
	}
}
