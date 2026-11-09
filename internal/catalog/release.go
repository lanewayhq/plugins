package catalog

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
)

// ManifestFile is a Plugin's manifest, at the top of its directory and of
// its release archive.
const ManifestFile = "plugin.json"

// defaultModule is a Plugin's WASM module when its manifest doesn't name one.
const defaultModule = "plugin.wasm"

// Manifest is what publishing needs from a Plugin's manifest: Laneway checks
// the rest when it installs the Plugin.
type Manifest struct {
	Name         string     `json:"name"`
	Version      string     `json:"version"`
	APIVersion   int        `json:"api_version"`
	Module       string     `json:"module"`
	Recipes      []Detector `json:"recipes"`
	Integrations []Detector `json:"integrations"`
	// EngineFiles are its Engine manifests, relative to its directory.
	EngineFiles []string          `json:"engines"`
	Commands    []json.RawMessage `json:"commands"`
	Generators  []json.RawMessage `json:"generators"`
	Hooks       []string          `json:"hooks"`
}

// ParseManifest reads a manifest.
func ParseManifest(data []byte) (Manifest, error) {
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return Manifest{}, fmt.Errorf("%s isn't a valid manifest: %w", ManifestFile, err)
	}
	if !name.MatchString(m.Name) {
		return Manifest{}, fmt.Errorf("%s: %q isn't a Plugin name", ManifestFile, m.Name)
	}
	if _, ok := parseVersion(m.Version); !ok {
		return Manifest{}, fmt.Errorf("%s: version %q isn't a version such as \"1.2.0\"", ManifestFile, m.Version)
	}
	return m, nil
}

// ModuleFile is the Plugin's WASM module, relative to its directory, or ""
// for a Plugin with no calls to answer, such as one that only adds Engines.
func (m Manifest) ModuleFile() string {
	if len(m.Recipes) == 0 && len(m.Integrations) == 0 && len(m.Commands) == 0 && len(m.Generators) == 0 && len(m.Hooks) == 0 {
		return ""
	}
	if m.Module == "" {
		return defaultModule
	}
	return m.Module
}

// Files are what a release of the Plugin holds, relative to its directory:
// its manifest, its module and its Engines' manifests. Its source and test
// cases stay in the repository.
func (m Manifest) Files() []string {
	files := []string{ManifestFile}
	if module := m.ModuleFile(); module != "" {
		files = append(files, module)
	}
	return append(files, m.EngineFiles...)
}

// ArchiveName is the file name of the release archive of version of the
// Plugin name.
func ArchiveName(name, version string) string {
	return fmt.Sprintf("%s-%s.tar.gz", name, version)
}

// Tag is the git tag that releases version of the Plugin name.
func Tag(name, version string) string { return name + "/v" + version }

// ParseTag splits a release tag, such as "laravel/v1.2.0", into the Plugin's
// name and version.
func ParseTag(tag string) (name, version string, err error) {
	n, v, ok := strings.Cut(tag, "/v")
	if !ok || !nameRE(n) {
		return "", "", fmt.Errorf("%q isn't a release tag such as \"laravel/v1.2.0\"", tag)
	}
	if _, ok := parseVersion(v); !ok {
		return "", "", fmt.Errorf("%q isn't a release tag: %q isn't a version such as \"1.2.0\"", tag, v)
	}
	return n, v, nil
}

func nameRE(s string) bool { return name.MatchString(s) }

// PackDir makes the release archive of the Plugin in dir: a gzipped tar of
// Files, at the top of the archive, the way Laneway unpacks a release. It is
// reproducible: the same files make the same bytes, so a release's SHA-256
// only changes when what it holds does.
func PackDir(dir string) (Manifest, []byte, error) {
	data, err := os.ReadFile(filepath.Join(dir, ManifestFile))
	if err != nil {
		return Manifest{}, nil, err
	}
	m, err := ParseManifest(data)
	if err != nil {
		return Manifest{}, nil, fmt.Errorf("%s: %w", dir, err)
	}
	files := map[string][]byte{}
	for _, f := range m.Files() {
		if path.IsAbs(f) || !validPath(f) {
			return Manifest{}, nil, fmt.Errorf("%s: %q isn't a path inside the Plugin's directory", dir, f)
		}
		data, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(f)))
		if errors.Is(err, os.ErrNotExist) && f == m.ModuleFile() {
			return Manifest{}, nil, fmt.Errorf("%s: its module %s isn't built; run `go run ./cmd/plugins build %s`", dir, f, m.Name)
		}
		if err != nil {
			return Manifest{}, nil, err
		}
		files[f] = data
	}
	archive, err := pack(files)
	return m, archive, err
}

func validPath(p string) bool {
	clean := path.Clean(p)
	return clean == p && clean != "." && !strings.HasPrefix(clean, "../") && clean != ".."
}

// epoch is every archived file's modification time.
var epoch = time.Unix(0, 0).UTC()

// pack makes a reproducible gzipped tar of files (path → content), sorted
// by path, each a regular file readable by everyone.
func pack(files map[string][]byte) ([]byte, error) {
	var buf bytes.Buffer
	gz, err := gzip.NewWriterLevel(&buf, gzip.BestCompression)
	if err != nil {
		return nil, err
	}
	tw := tar.NewWriter(gz)
	names := make([]string, 0, len(files))
	for n := range files {
		names = append(names, n)
	}
	slices.Sort(names)
	for _, n := range names {
		h := &tar.Header{Typeflag: tar.TypeReg, Name: n, Mode: 0o644, Size: int64(len(files[n])), ModTime: epoch, Format: tar.FormatPAX}
		if err := tw.WriteHeader(h); err != nil {
			return nil, err
		}
		if _, err := tw.Write(files[n]); err != nil {
			return nil, err
		}
	}
	if err := tw.Close(); err != nil {
		return nil, err
	}
	if err := gz.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// ReadArchive reads the regular files in a release archive, by path.
func ReadArchive(archive []byte) (map[string][]byte, error) {
	gz, err := gzip.NewReader(bytes.NewReader(archive))
	if err != nil {
		return nil, err
	}
	tr := tar.NewReader(gz)
	files := map[string][]byte{}
	for {
		h, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return files, nil
		}
		if err != nil {
			return nil, err
		}
		if h.Typeflag != tar.TypeReg {
			continue
		}
		data, err := io.ReadAll(tr)
		if err != nil {
			return nil, err
		}
		files[path.Clean(h.Name)] = data
	}
}

// ReleaseOf describes the release archive as the Catalog lists it, with url
// as where it's downloaded from: its version, Plugin API version, SHA-256 and
// what it adds, read from the manifest and Engine manifests inside it. It
// also returns the Plugin's name.
func ReleaseOf(archive []byte, url string) (string, Release, error) {
	files, err := ReadArchive(archive)
	if err != nil {
		return "", Release{}, err
	}
	data, ok := files[ManifestFile]
	if !ok {
		return "", Release{}, fmt.Errorf("it has no %s at its top", ManifestFile)
	}
	m, err := ParseManifest(data)
	if err != nil {
		return "", Release{}, err
	}
	if module := m.ModuleFile(); module != "" {
		if _, ok := files[path.Clean(module)]; !ok {
			return "", Release{}, fmt.Errorf("it has no module %s", module)
		}
	}
	r := Release{Version: m.Version, APIVersion: m.APIVersion, URL: url, SHA256: SHA256(archive),
		Recipes: m.Recipes, Integrations: m.Integrations}
	for _, f := range m.EngineFiles {
		data, ok := files[path.Clean(f)]
		if !ok {
			return "", Release{}, fmt.Errorf("it has no Engine manifest %s", f)
		}
		var e struct {
			Name string `toml:"name"`
		}
		if err := toml.Unmarshal(data, &e); err != nil {
			return "", Release{}, fmt.Errorf("%s: %w", f, err)
		}
		r.Engines = append(r.Engines, e.Name)
	}
	return m.Name, r, nil
}

// Build makes the Catalog listing releases (each a Plugin's name and a
// release), and encodes it as it is signed and served: Plugins by name,
// each one's releases oldest first, so the same releases always make the
// same bytes.
func Build(releases map[string][]Release) (*Catalog, []byte, error) {
	c := &Catalog{Format: Format}
	names := make([]string, 0, len(releases))
	for n := range releases {
		names = append(names, n)
	}
	slices.Sort(names)
	for _, n := range names {
		rs := slices.Clone(releases[n])
		slices.SortFunc(rs, func(a, b Release) int { return compareVersions(a.Version, b.Version) })
		for i := 1; i < len(rs); i++ {
			if compareVersions(rs[i-1].Version, rs[i].Version) == 0 {
				return nil, nil, fmt.Errorf("Plugin %s has two releases of version %s", n, rs[i].Version)
			}
		}
		c.Plugins = append(c.Plugins, Plugin{Name: n, Releases: rs})
	}
	if err := c.validate(); err != nil {
		return nil, nil, err
	}
	data, err := Encode(c)
	return c, data, err
}

// Encode is the Catalog's JSON, as it is signed and served.
func Encode(c *Catalog) ([]byte, error) {
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

// PlaceholderKey is the public key keys/catalog.pub holds until a
// maintainer replaces it with the real one: the same placeholder Laneway
// builds in (placeholderKey in its internal/catalog/fetch.go). Nobody holds
// its private key.
const PlaceholderKey = "TEFORVdBWS1QTEFDRUhPTERFUi1LRVktU0VFLSM3MSE="

// ErrPlaceholderKey is returned for the placeholder public key.
var ErrPlaceholderKey = errors.New("it is the placeholder key, not the Catalog's real key; see keys/README.md")

// ParsePublicKey reads a public key file: the base64-encoded Ed25519 public
// key on the first line that isn't blank or a # comment, as Laneway builds
// it in.
func ParsePublicKey(data []byte) (ed25519.PublicKey, error) {
	text := keyLine(data)
	key, err := base64.StdEncoding.DecodeString(text)
	if err != nil || len(key) != ed25519.PublicKeySize {
		return nil, errors.New("it isn't a base64-encoded Ed25519 public key")
	}
	if text == PlaceholderKey {
		return key, ErrPlaceholderKey
	}
	return key, nil
}

// ParsePrivateKey reads a private key file: the base64-encoded 32-byte
// Ed25519 seed, as Keygen writes it.
func ParsePrivateKey(data []byte) (ed25519.PrivateKey, error) {
	seed, err := base64.StdEncoding.DecodeString(keyLine(data))
	if err != nil || len(seed) != ed25519.SeedSize {
		return nil, errors.New("it isn't a base64-encoded Ed25519 private key (a 32-byte seed)")
	}
	return ed25519.NewKeyFromSeed(seed), nil
}

// EncodeKeys returns the files of a key pair, as ParsePrivateKey and
// ParsePublicKey read them.
func EncodeKeys(key ed25519.PrivateKey) (private, public []byte) {
	pub := key.Public().(ed25519.PublicKey)
	private = []byte("# Laneway Catalog signing key (Ed25519 seed). Keep it offline; never commit it.\n" +
		base64.StdEncoding.EncodeToString(key.Seed()) + "\n")
	public = []byte(base64.StdEncoding.EncodeToString(pub) + "\n")
	return private, public
}

func keyLine(data []byte) string {
	for line := range strings.SplitSeq(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line != "" && !strings.HasPrefix(line, "#") {
			return line
		}
	}
	return ""
}

// SHA256 is data's hash as a Release lists it.
func SHA256(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
