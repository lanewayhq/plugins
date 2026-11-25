package agents

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// TestCommandsSucceedWithoutLane runs each command a Harness runs where
// `lane` isn't installed, as on a teammate's machine without Laneway: it
// succeeds, saying only what the developer needs to know.
func TestCommandsSucceedWithoutLane(t *testing.T) {
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("no sh")
	}
	// A PATH with sh, which the commands run under, and nothing else.
	bin := t.TempDir()
	if err := os.Symlink(sh, filepath.Join(bin, "sh")); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		name, command, output string
	}{
		{"ReadyCommand", ReadyCommand, ""},
		{"OpenCommand", OpenCommand, "Laneway isn't installed, so this worktree has no URL to open\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			cmd := exec.Command(sh, "-c", c.command)
			cmd.Env = []string{"PATH=" + bin}
			out, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("%s failed without lane: %v\n%s", c.name, err, out)
			}
			if string(out) != c.output {
				t.Errorf("%s said %q without lane, want %q", c.name, out, c.output)
			}
		})
	}
}
