package catalog

import (
	"bytes"
	"crypto/ed25519"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// plugin writes a Plugin's files (path → content) into a new directory.
func plugin(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, content := range files {
		path := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

const engineManifest = `{"name": "kv", "version": "1.2.0", "api_version": 1, "engines": ["kv.toml"]}`

func TestAReleaseHoldsTheManifestModuleAndEnginesOnlyAndIsReproducible(t *testing.T) {
	dir := plugin(t, map[string]string{
		"plugin.json":            `{"name": "stack", "version": "0.3.0", "api_version": 1, "recipes": [{"name": "stack", "detect": {"file": "stack.yml"}}], "engines": ["db.toml"]}`,
		"plugin.wasm":            "\x00asm",
		"db.toml":                "name = \"stackdb\"\nimage = \"x\"\n",
		"main.go":                "package main",
		"testdata/a/project/x":   "",
		"testdata/a/golden.json": "{}",
	})

	m, archive, err := PackDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	files, err := ReadArchive(archive)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for n := range files {
		names = append(names, n)
	}
	slices.Sort(names)
	if want := []string{"db.toml", "plugin.json", "plugin.wasm"}; !slices.Equal(names, want) {
		t.Errorf("the release holds %q, want %q", names, want)
	}
	_, again, err := PackDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(archive, again) {
		t.Error("packing the same Plugin twice made different archives")
	}

	name, r, err := ReleaseOf(archive, "https://example.com/stack-0.3.0.tar.gz")
	if err != nil {
		t.Fatal(err)
	}
	if name != "stack" || r.Version != m.Version || r.APIVersion != 1 || r.SHA256 != SHA256(archive) ||
		len(r.Recipes) != 1 || r.Recipes[0].Detect.File != "stack.yml" || !slices.Equal(r.Engines, []string{"stackdb"}) {
		t.Errorf("ReleaseOf = %s %+v", name, r)
	}
}

func TestAPluginThatNeedsAModuleIsntPackedUntilItIsBuilt(t *testing.T) {
	dir := plugin(t, map[string]string{"plugin.json": `{"name": "gen", "version": "1.0.0", "api_version": 1, "generators": [{"name": "gen"}]}`})

	_, _, err := PackDir(dir)

	if err == nil || !strings.Contains(err.Error(), "isn't built") {
		t.Errorf("err = %v, want it to say the module isn't built", err)
	}
}

func TestTheCatalogListsPluginsByNameAndReleasesOldestFirstAndIsSignedAsLanewayReadsIt(t *testing.T) {
	_, kv, err := PackDir(plugin(t, map[string]string{"plugin.json": engineManifest, "kv.toml": "name = \"kv\"\n"}))
	if err != nil {
		t.Fatal(err)
	}
	_, r120, _ := ReleaseOf(kv, "kv-1.2.0.tar.gz")
	r100, r1100 := r120, r120
	r100.Version, r1100.Version = "1.0.0", "1.10.0"
	_, data, err := Build(map[string][]Release{"kv": {r1100, r120, r100}, "a-first": {r100}})
	if err != nil {
		t.Fatal(err)
	}
	_, key, _ := ed25519.GenerateKey(nil)

	c, err := Verify(data, Sign(data, key), key.Public().(ed25519.PublicKey))

	if err != nil {
		t.Fatal(err)
	}
	if c.Plugins[0].Name != "a-first" || c.Plugins[1].Name != "kv" {
		t.Errorf("Plugins = %+v, want them by name", c.Plugins)
	}
	var versions []string
	for _, r := range c.Plugins[1].Releases {
		versions = append(versions, r.Version)
	}
	if want := []string{"1.0.0", "1.2.0", "1.10.0"}; !slices.Equal(versions, want) {
		t.Errorf("kv's releases are %q, want %q", versions, want)
	}
	if latest := c.Latest(1)["kv"].Version; latest != "1.10.0" {
		t.Errorf("latest kv = %s", latest)
	}
	_, again, _ := Build(map[string][]Release{"a-first": {r100}, "kv": {r100, r1100, r120}})
	if !bytes.Equal(data, again) {
		t.Error("the same releases built a different Catalog")
	}
}

func TestTwoReleasesOfOneVersionAreRefused(t *testing.T) {
	r := Release{Version: "1.0.0", APIVersion: 1, URL: "x", SHA256: strings.Repeat("a", 64)}
	if _, _, err := Build(map[string][]Release{"kv": {r, r}}); err == nil {
		t.Error("Build accepted two releases of kv 1.0.0")
	}
}

func TestReleaseTags(t *testing.T) {
	if n, v, err := ParseTag("laravel/v1.2.0"); err != nil || n != "laravel" || v != "1.2.0" {
		t.Errorf("ParseTag(laravel/v1.2.0) = %q %q %v", n, v, err)
	}
	for _, bad := range []string{"v1.2.0", "laravel/1.2.0", "Laravel/v1.2.0", "laravel/vx", "catalog/20261109"} {
		if _, _, err := ParseTag(bad); err == nil {
			t.Errorf("ParseTag(%q) accepted it", bad)
		}
	}
	if Tag("laravel", "1.2.0") != "laravel/v1.2.0" {
		t.Error("Tag isn't ParseTag's opposite")
	}
}

func TestKeysRoundTripAndThePlaceholderIsNamed(t *testing.T) {
	_, key, _ := ed25519.GenerateKey(nil)
	private, public := EncodeKeys(key)
	gotKey, err := ParsePrivateKey(private)
	if err != nil || !gotKey.Equal(key) {
		t.Errorf("ParsePrivateKey = %v, %v", gotKey, err)
	}
	pub, err := ParsePublicKey(append([]byte("# a comment\n\n"), public...))
	if err != nil || !pub.Equal(key.Public()) {
		t.Errorf("ParsePublicKey = %v, %v", pub, err)
	}
	if _, err := ParsePublicKey([]byte("# PLACEHOLDER\n" + PlaceholderKey + "\n")); !errors.Is(err, ErrPlaceholderKey) {
		t.Errorf("the placeholder key: err = %v, want ErrPlaceholderKey", err)
	}
	if _, err := ParsePublicKey([]byte("not a key")); err == nil {
		t.Error("ParsePublicKey accepted garbage")
	}
}

func TestTheCommittedPublicKeyFileParses(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "keys", "catalog.pub"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ParsePublicKey(data); err != nil && !errors.Is(err, ErrPlaceholderKey) {
		t.Errorf("keys/catalog.pub: %v", err)
	}
}
