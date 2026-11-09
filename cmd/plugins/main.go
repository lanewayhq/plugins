// Command plugins builds, tests and releases the first-party Plugins in
// plugins/, and builds, signs and verifies the Catalog that lists their
// releases. Run it from the repository's root:
//
//	go run ./cmd/plugins build [plugin...]       build each Plugin's WASM module
//	go run ./cmd/plugins test [-update] [plugin...]
//	                                             run `lane plugin test` for each Plugin
//	go run ./cmd/plugins pack [-tag name/vX.Y.Z] [-out dist] plugin
//	                                             write a Plugin's release files
//	go run ./cmd/plugins catalog -releases dir [-base-url url] [-o catalog/catalog.json]
//	                                             build the Catalog from release archives
//	go run ./cmd/plugins sign -key file [catalog/catalog.json]
//	                                             sign the Catalog (offline key)
//	go run ./cmd/plugins verify [-pub keys/catalog.pub] [catalog/catalog.json]
//	                                             check the Catalog's signature
//	go run ./cmd/plugins keygen -out dir         make a new signing key pair
//	go run ./cmd/plugins list                    list the Plugins, one per line
//
// With no plugin named, build and test act on every Plugin.
package main

import (
	"bytes"
	"crypto/ed25519"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"

	"github.com/lanewayhq/plugins/internal/catalog"
)

// pluginsDir holds one directory per Plugin, named after it.
const pluginsDir = "plugins"

// DefaultBaseURL is where releases are downloaded from: a release's archive
// is at <base>/<tag>/<archive>, the GitHub release of its tag.
const DefaultBaseURL = "https://github.com/lanewayhq/plugins/releases/download"

var commands = map[string]func(args []string) error{
	"build":   runBuild,
	"test":    runTest,
	"pack":    runPack,
	"catalog": runCatalog,
	"sign":    runSign,
	"verify":  runVerify,
	"keygen":  runKeygen,
	"list":    runList,
}

func main() {
	if len(os.Args) < 2 || commands[os.Args[1]] == nil {
		fmt.Fprintln(os.Stderr, "usage: go run ./cmd/plugins build|test|pack|catalog|sign|verify|keygen|list ... (see cmd/plugins/main.go)")
		os.Exit(2)
	}
	if err := commands[os.Args[1]](os.Args[2:]); err != nil {
		fmt.Fprintf(os.Stderr, "plugins %s: %v\n", os.Args[1], err)
		os.Exit(1)
	}
}

// allPlugins are the names of the Plugins in pluginsDir.
func allPlugins() ([]string, error) {
	manifests, err := filepath.Glob(filepath.Join(pluginsDir, "*", catalog.ManifestFile))
	if err != nil {
		return nil, err
	}
	var names []string
	for _, m := range manifests {
		names = append(names, filepath.Base(filepath.Dir(m)))
	}
	if len(names) == 0 {
		return nil, fmt.Errorf("no Plugins in %s/; run this from the repository's root", pluginsDir)
	}
	slices.Sort(names)
	return names, nil
}

// selected are the Plugins named, or every Plugin.
func selected(names []string) ([]string, error) {
	all, err := allPlugins()
	if err != nil {
		return nil, err
	}
	if len(names) == 0 {
		return all, nil
	}
	for _, n := range names {
		if !slices.Contains(all, n) {
			return nil, fmt.Errorf("there is no Plugin %s in %s/", n, pluginsDir)
		}
	}
	return names, nil
}

func runList(args []string) error {
	names, err := allPlugins()
	if err != nil {
		return err
	}
	fmt.Println(strings.Join(names, "\n"))
	return nil
}

func runBuild(args []string) error {
	names, err := selected(args)
	if err != nil {
		return err
	}
	for _, n := range names {
		if err := build(n); err != nil {
			return err
		}
	}
	return nil
}

// build compiles the module of the Plugin name, if it has one (Go source
// beside its manifest), as a WASI command at the path its manifest names.
// The build is reproducible (no paths, no VCS stamp, no debug info), so a
// module only changes when its source or the Go toolchain does.
func build(name string) error {
	dir := filepath.Join(pluginsDir, name)
	data, err := os.ReadFile(filepath.Join(dir, catalog.ManifestFile))
	if err != nil {
		return err
	}
	m, err := catalog.ParseManifest(data)
	if err != nil {
		return fmt.Errorf("%s: %w", dir, err)
	}
	module := m.ModuleFile()
	if module == "" {
		return nil
	}
	if src, _ := filepath.Glob(filepath.Join(dir, "*.go")); len(src) == 0 {
		return fmt.Errorf("%s: its manifest needs a module, but it has no Go source to build one from", dir)
	}
	out, err := filepath.Abs(filepath.Join(dir, filepath.FromSlash(module)))
	if err != nil {
		return err
	}
	cmd := exec.Command("go", "build", "-trimpath", "-buildvcs=false", "-ldflags=-s -w", "-o", out, "./"+filepath.ToSlash(dir))
	cmd.Env = append(os.Environ(), "GOOS=wasip1", "GOARCH=wasm", "CGO_ENABLED=0")
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("building %s: %v\n%s", dir, err, output)
	}
	return nil
}

func runTest(args []string) error {
	fs := flag.NewFlagSet("test", flag.ExitOnError)
	lane := fs.String("lane", envOr("LANE", "lane"), "the `lane` binary to test with ($LANE)")
	update := fs.Bool("update", false, "rewrite the golden files with what each Plugin returns now")
	fs.Parse(args)
	names, err := selected(fs.Args())
	if err != nil {
		return err
	}
	var failed []string
	for _, n := range names {
		if err := build(n); err != nil {
			return err
		}
		fmt.Printf("== %s\n", n)
		cmdArgs := []string{"plugin", "test", filepath.Join(pluginsDir, n)}
		if *update {
			cmdArgs = append(cmdArgs, "--update")
		}
		cmd := exec.Command(*lane, cmdArgs...)
		cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
		if err := cmd.Run(); err != nil {
			var exit *exec.ExitError
			if !errors.As(err, &exit) {
				return fmt.Errorf("running %s: %w", *lane, err)
			}
			failed = append(failed, n)
		}
	}
	if len(failed) > 0 {
		return fmt.Errorf("`lane plugin test` failed for %s", strings.Join(failed, ", "))
	}
	return nil
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// runPack writes the release files of one Plugin into the output directory:
// its archive (what Laneway downloads, and the Catalog lists the SHA-256
// of), its manifest beside it, and a SHA256SUMS file.
func runPack(args []string) error {
	fs := flag.NewFlagSet("pack", flag.ExitOnError)
	out := fs.String("out", "dist", "`directory` to write the release files to")
	tag := fs.String("tag", "", "the release `tag`, such as laravel/v1.2.0; it must match the Plugin's manifest")
	fs.Parse(args)
	if fs.NArg() != 1 {
		return errors.New("name one Plugin, such as `pack laravel`")
	}
	name := fs.Arg(0)
	if _, err := selected([]string{name}); err != nil {
		return err
	}
	if err := build(name); err != nil {
		return err
	}
	m, archive, err := catalog.PackDir(filepath.Join(pluginsDir, name))
	if err != nil {
		return err
	}
	if *tag != "" {
		tn, tv, err := catalog.ParseTag(*tag)
		if err != nil {
			return err
		}
		if tn != m.Name || tv != m.Version {
			return fmt.Errorf("the tag %s releases %s %s, but %s/%s/%s is %s %s; bump version in the manifest, or tag %s",
				*tag, tn, tv, pluginsDir, name, catalog.ManifestFile, m.Name, m.Version, catalog.Tag(m.Name, m.Version))
		}
	}
	if err := os.MkdirAll(*out, 0o755); err != nil {
		return err
	}
	file := catalog.ArchiveName(m.Name, m.Version)
	manifest, err := os.ReadFile(filepath.Join(pluginsDir, name, catalog.ManifestFile))
	if err != nil {
		return err
	}
	sums := fmt.Sprintf("%s  %s\n", catalog.SHA256(archive), file)
	for f, data := range map[string][]byte{file: archive, catalog.ManifestFile: manifest, "SHA256SUMS": []byte(sums)} {
		if err := os.WriteFile(filepath.Join(*out, f), data, 0o644); err != nil {
			return err
		}
	}
	fmt.Printf("%s %s: %s (sha256 %s), built with %s\n", m.Name, m.Version, filepath.Join(*out, file), catalog.SHA256(archive), runtime.Version())
	return nil
}

// runCatalog builds the Catalog from every release archive in a directory
// (any depth), each listed at <base-url>/<tag>/<archive>.
func runCatalog(args []string) error {
	fs := flag.NewFlagSet("catalog", flag.ExitOnError)
	dir := fs.String("releases", "", "`directory` holding the release archives (<plugin>-<version>.tar.gz)")
	base := fs.String("base-url", DefaultBaseURL, "`url` releases are downloaded from: an archive is at <url>/<plugin>/v<version>/<archive>")
	out := fs.String("o", filepath.Join("catalog", "catalog.json"), "`file` to write the Catalog to")
	fs.Parse(args)
	if *dir == "" {
		return errors.New("-releases is required")
	}
	releases := map[string][]catalog.Release{}
	err := filepath.WalkDir(*dir, func(p string, e os.DirEntry, err error) error {
		if err != nil || e.IsDir() || !strings.HasSuffix(p, ".tar.gz") {
			return err
		}
		archive, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		name, r, err := catalog.ReleaseOf(archive, "")
		if err != nil {
			return fmt.Errorf("%s: %w", p, err)
		}
		file := catalog.ArchiveName(name, r.Version)
		if filepath.Base(p) != file {
			return fmt.Errorf("%s holds %s %s, which is released as %s", p, name, r.Version, file)
		}
		r.URL = strings.TrimSuffix(*base, "/") + "/" + catalog.Tag(name, r.Version) + "/" + file
		releases[name] = append(releases[name], r)
		return nil
	})
	if err != nil {
		return err
	}
	c, data, err := catalog.Build(releases)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(*out), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(*out, data, 0o644); err != nil {
		return err
	}
	n := 0
	for _, p := range c.Plugins {
		n += len(p.Releases)
	}
	fmt.Printf("wrote %s: %d Plugins, %d releases\n", *out, len(c.Plugins), n)
	return nil
}

func catalogArg(fs *flag.FlagSet) string {
	if fs.NArg() > 0 {
		return fs.Arg(0)
	}
	return filepath.Join("catalog", "catalog.json")
}

// runSign signs the Catalog with the private key, writing the signature
// beside it (catalog.json.sig), as Laneway downloads it. Run it offline.
func runSign(args []string) error {
	fs := flag.NewFlagSet("sign", flag.ExitOnError)
	keyFile := fs.String("key", "", "the private key `file` (see keygen)")
	fs.Parse(args)
	if *keyFile == "" {
		return errors.New("-key is required: the Catalog's private key, kept offline")
	}
	file := catalogArg(fs)
	keyData, err := os.ReadFile(*keyFile)
	if err != nil {
		return err
	}
	key, err := catalog.ParsePrivateKey(keyData)
	if err != nil {
		return fmt.Errorf("%s: %w", *keyFile, err)
	}
	data, err := os.ReadFile(file)
	if err != nil {
		return err
	}
	// Refuse to sign what Laneway wouldn't read.
	sig := catalog.Sign(data, key)
	if _, err := catalog.Verify(data, sig, key.Public().(ed25519.PublicKey)); err != nil {
		return fmt.Errorf("%s: %w", file, err)
	}
	if err := os.WriteFile(file+catalog.SignatureSuffix, sig, 0o644); err != nil {
		return err
	}
	fmt.Printf("signed %s: %s\n", file, file+catalog.SignatureSuffix)
	return nil
}

// runVerify checks the Catalog's signature against the public key, the way
// Laneway does, and that it is a Catalog Laneway reads.
func runVerify(args []string) error {
	fs := flag.NewFlagSet("verify", flag.ExitOnError)
	pubFile := fs.String("pub", filepath.Join("keys", "catalog.pub"), "the public key `file`")
	fs.Parse(args)
	file := catalogArg(fs)
	pubData, err := os.ReadFile(*pubFile)
	if err != nil {
		return err
	}
	pub, err := catalog.ParsePublicKey(pubData)
	if err != nil {
		return fmt.Errorf("%s: %w", *pubFile, err)
	}
	data, err := os.ReadFile(file)
	if err != nil {
		return err
	}
	sig, err := os.ReadFile(file + catalog.SignatureSuffix)
	if err != nil {
		return err
	}
	c, err := catalog.Verify(data, sig, pub)
	if err != nil {
		return fmt.Errorf("%s: Laneway would refuse it: %w", file, err)
	}
	fmt.Printf("%s is signed with %s: %d Plugins\n", file, *pubFile, len(c.Plugins))
	return nil
}

// runKeygen makes a new key pair: catalog.key (private, readable only by
// its owner) and catalog.pub in the output directory, refusing to overwrite
// either.
func runKeygen(args []string) error {
	fs := flag.NewFlagSet("keygen", flag.ExitOnError)
	out := fs.String("out", "", "`directory` to write catalog.key and catalog.pub to")
	fs.Parse(args)
	if *out == "" {
		return errors.New("-out is required")
	}
	_, key, err := ed25519.GenerateKey(nil)
	if err != nil {
		return err
	}
	private, public := catalog.EncodeKeys(key)
	if err := os.MkdirAll(*out, 0o700); err != nil {
		return err
	}
	for _, f := range []struct {
		name string
		data []byte
		mode os.FileMode
	}{{"catalog.key", private, 0o600}, {"catalog.pub", public, 0o644}} {
		w, err := os.OpenFile(filepath.Join(*out, f.name), os.O_WRONLY|os.O_CREATE|os.O_EXCL, f.mode)
		if err != nil {
			return err
		}
		if _, err := w.Write(f.data); err != nil {
			w.Close()
			return err
		}
		if err := w.Close(); err != nil {
			return err
		}
	}
	fmt.Printf("wrote %s (private: keep it offline) and %s:\n%s", filepath.Join(*out, "catalog.key"), filepath.Join(*out, "catalog.pub"), bytes.TrimSpace(public))
	fmt.Println()
	return nil
}
