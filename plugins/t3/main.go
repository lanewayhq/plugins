// Command t3 is the t3 Plugin: the T3 Code Generator, run by `lane generate
// t3` (and `lane harness add t3`). It merges two scripts into t3.json, one
// that holds T3 Code's agent until the Worktree is Ready and one that opens
// the Worktree in the browser, and the agent instructions into AGENTS.md or
// Laravel Boost's guidelines.
package main

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/lanewayhq/plugins/internal/agents"
	"github.com/lanewayhq/plugins/sdk"
)

func main() {
	sdk.Run(sdk.Plugin{Generators: map[string]sdk.Generator{"t3": generate}})
}

// t3File is T3 Code's committed project file.
const t3File = "t3.json"

// t3Schema is the JSON Schema a new t3.json points at, as T3 Code writes it.
const t3Schema = "https://t3.codes/schema/t3.json"

// t3Script is a T3 Code script, its keys in the order T3 Code writes them.
// T3 Code reads Async only for a runOnWorktreeCreate script, and it defaults
// to true, so it is set only there.
type t3Script struct {
	Name                string `json:"name"`
	Command             string `json:"command"`
	Icon                string `json:"icon"`
	RunOnWorktreeCreate bool   `json:"runOnWorktreeCreate"`
	Async               *bool  `json:"async,omitempty"`
}

// lanewayScripts are the scripts Laneway adds to t3.json. A script's name is
// what T3 Code's scripts menu shows, and how Laneway recognises its own
// script on later runs (t3.json allows no keys of our own).
var lanewayScripts = []t3Script{
	{Name: "Laneway: wait until Ready", Command: agents.ReadyCommand, Icon: "configure", RunOnWorktreeCreate: true, Async: new(false)},
	// Merging sets only the keys given, so an older open script's async is
	// left in place. That's harmless: T3 Code reads async only for a
	// runOnWorktreeCreate script, and this one's runOnWorktreeCreate is off.
	{Name: "Laneway: open in browser", Command: agents.OpenCommand, Icon: "play"},
}

// isLanewayScript reports whether name is one of lanewayScripts' names.
func isLanewayScript(name string) bool {
	return slices.ContainsFunc(lanewayScripts, func(s t3Script) bool { return s.Name == name })
}

// generate is the T3 Code Generator. It adds two scripts to t3.json: one
// that T3 Code runs in every Worktree it creates for a thread
// (runOnWorktreeCreate), holding the agent until it exits (async: false),
// and one the developer runs from the scripts menu to open the thread's
// Worktree in the browser. It doesn't set previewUrl: t3.json is committed,
// so every Worktree's thread would preview the same URL. It also adds the
// agent instructions to AGENTS.md, which the agents T3 Code runs read (or,
// with Laravel Boost, to Boost's guidelines).
func generate(p sdk.Project) (sdk.GeneratorResult, error) {
	// Laneway owns the script in t3.json's scripts with each script's name:
	// it is added once, and kept up to date on later runs.
	var entries []sdk.JSONEntry
	for _, script := range lanewayScripts {
		entries = append(entries, sdk.JSONEntry{At: []string{"scripts"}, Match: map[string]any{"name": script.Name}, Value: script})
	}
	effects := []sdk.Effect{sdk.File(sdk.FileBlock{Path: t3File, JSON: &sdk.JSONBlock{
		New:     map[string]any{"$schema": t3Schema},
		Entries: entries,
	}})}
	if others := otherWorktreeCreateScripts(p); len(others) > 0 {
		effects = append(effects, sdk.Warning(fmt.Sprintf(
			"%s: T3 Code only runs the first script with runOnWorktreeCreate, and %s %s too, so they won't all run; "+
				"run `lane worktree ready` at the end of yours instead, or turn runOnWorktreeCreate off on all but one",
			t3File, strings.Join(others, ", "), plural(len(others), "has it", "have it"))))
	}
	instructions, err := agents.InstructionEffects(p, false)
	if err != nil {
		return sdk.GeneratorResult{}, err
	}
	return sdk.GeneratorResult{Effects: append(effects, instructions...)}, nil
}

// otherWorktreeCreateScripts returns the quoted names of the scripts in the
// Worktree's t3.json, other than Laneway's, that run on worktree creation.
// That leaves out an older open script with runOnWorktreeCreate on, since
// this run's merge turns it off. A t3.json Laneway can't read has none:
// merging into it fails anyway.
func otherWorktreeCreateScripts(p sdk.Project) []string {
	data, err := p.ReadFile(t3File)
	if err != nil {
		return nil
	}
	var doc struct {
		Scripts []struct {
			Name                string `json:"name"`
			RunOnWorktreeCreate any    `json:"runOnWorktreeCreate"`
		} `json:"scripts"`
	}
	if json.Unmarshal(data, &doc) != nil {
		return nil
	}
	var names []string
	for _, s := range doc.Scripts {
		if on, _ := s.RunOnWorktreeCreate.(bool); on && !isLanewayScript(s.Name) {
			names = append(names, fmt.Sprintf("%q", s.Name))
		}
	}
	return names
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}
