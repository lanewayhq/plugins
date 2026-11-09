// Command claude is the claude Plugin: the Claude Code Generator, run by
// `lane generate claude` (and `lane harness add claude`). It merges a
// SessionStart hook that waits until the Worktree is Ready into Claude Code's
// project settings, and the agent instructions into AGENTS.md or Laravel
// Boost's guidelines.
package main

import (
	"github.com/lanewayhq/plugins/internal/agents"
	"github.com/lanewayhq/plugins/sdk"
)

func main() {
	sdk.Run(sdk.Plugin{Generators: map[string]sdk.Generator{"claude": generate}})
}

// claudeSettings is Claude Code's shared (committed) project settings file.
const claudeSettings = ".claude/settings.json"

// claudeStatusMessage is the spinner text of the generated SessionStart hook,
// and how Laneway recognises its own hook on later runs.
const claudeStatusMessage = "Laneway: waiting for this Worktree to be Ready (from `lane harness add claude`)"

// claudeHandler is a Claude Code hook handler, its keys in the order Claude
// Code's docs write them.
type claudeHandler struct {
	Type          string `json:"type"`
	Command       string `json:"command"`
	Timeout       int    `json:"timeout"`
	StatusMessage string `json:"statusMessage"`
}

// generate is the Claude Code Generator. It adds a SessionStart hook to Claude
// Code's project settings that runs agents.ReadyCommand: SessionStart hooks run on
// startup, resume, /clear and compaction; Claude's first response waits for
// them, and a hook's stdout (the Worktree's URL) becomes part of the agent's
// context. Laneway's hook is the one with its status message, wherever it
// is, so it is repaired in place, or else added as a group of its own. It
// also adds the agent instructions to AGENTS.md, and imports them into
// CLAUDE.md.
func generate(p sdk.Project) (sdk.GeneratorResult, error) {
	handler := claudeHandler{Type: "command", Command: agents.ReadyCommand, Timeout: agents.ReadyTimeout, StatusMessage: claudeStatusMessage}
	settings := sdk.File(sdk.FileBlock{Path: claudeSettings, JSON: &sdk.JSONBlock{Entries: []sdk.JSONEntry{{
		At:    []string{"hooks", "SessionStart"},
		Match: map[string]any{"hooks": []any{map[string]any{"statusMessage": claudeStatusMessage}}},
		Value: map[string]any{"hooks": []any{handler}},
	}}}})
	instructions, err := agents.InstructionEffects(p, true)
	if err != nil {
		return sdk.GeneratorResult{}, err
	}
	return sdk.GeneratorResult{Effects: append([]sdk.Effect{settings}, instructions...)}, nil
}
