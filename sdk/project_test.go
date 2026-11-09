package sdk

import (
	"maps"
	"os"
	"path/filepath"
	"testing"
)

func TestServicesAreWhatTheProjectsConfigDeclaresWithLanewayLocalTomlWinning(t *testing.T) {
	for _, tc := range []struct {
		name  string
		files map[string]string
		want  map[string]string
	}{
		{"no config", nil, map[string]string{}},
		{"laneway.toml", map[string]string{"laneway.toml": "[services]\nrustfs = \"1\"\nredis = \"7\"\n"},
			map[string]string{"rustfs": "1", "redis": "7"}},
		{"laneway.local.toml adds and overrides", map[string]string{
			"laneway.toml":       "[services]\nredis = \"7\"\n",
			"laneway.local.toml": "[services]\nredis = \"8\"\nrustfs = \"1\"\n",
		}, map[string]string{"rustfs": "1", "redis": "8"}},
		{"invalid laneway.toml", map[string]string{
			"laneway.toml":       "[services]\nredis = 7\n",
			"laneway.local.toml": "[services]\nrustfs = \"1\"\n",
		}, map[string]string{"rustfs": "1"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			for name, data := range tc.files {
				if err := os.WriteFile(filepath.Join(dir, name), []byte(data), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			if got := (Project{Dir: dir}).Services(); !maps.Equal(got, tc.want) {
				t.Errorf("Services() = %v, want %v", got, tc.want)
			}
		})
	}
}
