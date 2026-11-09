// Package agents is what the Harness Generators (the claude and t3 Plugins)
// and the Laravel Boost Generator (the boost Plugin) share: the agent
// instructions they put where a Project's agents read them, and the command
// Harness hooks run to wait until a Worktree is Ready.
//
// Generated files are committed, so everything generated must be harmless
// for teammates without Laneway (see ReadyCommand), and Laneway merges it
// into the developer's files without overwriting their own.
package agents

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/lanewayhq/plugins/sdk"
)

// ReadyCommand is the shell command Harness hooks run: `lane worktree
// ready`, guarded so that it succeeds quietly when `lane` isn't installed
// or there is nothing to wait for (exit 3: not a Linked repository, or no
// daemon). It still fails when setup Failed (1) or waiting gave up (4).
// It runs under `sh` because T3 Code types its scripts into the developer's
// interactive shell, which may not be POSIX and where `exit` would close the
// terminal.
const ReadyCommand = `sh -c 'command -v lane >/dev/null 2>&1 || exit 0; lane worktree ready || [ $? -eq 3 ]'`

// ReadyTimeout is how long a Harness may let ReadyCommand run, in seconds.
// It is longer than `lane worktree ready`'s own 10 minute timeout, so that
// Laneway gives up first and says why.
const ReadyTimeout = 660

// The files agents read their instructions from.
const (
	AgentsFile = "AGENTS.md"
	ClaudeFile = "CLAUDE.md"
)

// Block is what every generator puts between the markers in AGENTS.md.
// It is static: the Worktree's own details come from `lane info --json`.
const Block = `## Local environment (Laneway)

This repository is served by Laneway when it's installed. Every git worktree gets its own HTTPS URL, database and ` + "`.env`" + `, so work only with this worktree's.

- Find this worktree's URL, its Processes and their states, its databases and its Service connection details with ` + "`lane info --json`" + `. Use that URL rather than starting your own server or guessing a port.
- Run the project's commands (tests, artisan, composer, npm…) with ` + "`lane exec <command>`" + `, such as ` + "`lane exec php artisan test`" + `, so they use this worktree's PHP and Node versions.
- ` + "`lane worktree ready`" + ` waits until this worktree is Ready (Laneway prepares each new worktree: its database, ` + "`.env`" + ` and setup commands). If it reports that preparing it failed, show the user its output; ` + "`lane worktree ready --retry`" + ` prepares it again once the problem is fixed.
- Name a linked worktree after your task: if ` + "`worktree.main`" + ` in ` + "`lane info --json`" + ` is false and its ` + "`aliases`" + ` is empty, then once you know your task and ` + "`lane worktree ready`" + ` has finished, run ` + "`lane worktree alias <name>`" + ` with a short kebab-case name for the task (lowercase letters, digits and ` + "`-`" + `, such as ` + "`checkout-redesign`" + `). If the name is taken, pick another. The Alias becomes this worktree's URL, so from then on use the ` + "`url`" + ` that ` + "`lane info --json`" + ` reports. Never add an Alias in the main worktree.
- If there is no ` + "`lane`" + ` command, Laneway isn't installed: ignore this section.
`

// claudeImport is the marked block added to CLAUDE.md: Claude Code reads
// CLAUDE.md, not AGENTS.md, and `@path` on its own line imports a file.
const claudeImport = "@" + AgentsFile + "\n"

// importsAgents matches an existing import of AGENTS.md in CLAUDE.md.
var importsAgents = regexp.MustCompile(`(^|\s)@(\./)?AGENTS\.md(\s|$)`)

// InstructionEffects add the agent instructions where the Project's agents
// read them: Laravel Boost's guidelines if the Project uses Boost, else
// AGENTS.md, imported into CLAUDE.md if importIntoClaude.
func InstructionEffects(p sdk.Project, importIntoClaude bool) ([]sdk.Effect, error) {
	if UsesBoost(p) {
		return BoostEffects(p), nil
	}
	effects := []sdk.Effect{sdk.File(sdk.Markdown(AgentsFile, Block))}
	if importIntoClaude && needsClaudeImport(p) {
		effects = append(effects, sdk.File(sdk.Markdown(ClaudeFile, claudeImport)))
	}
	return effects, nil
}

// needsClaudeImport reports whether CLAUDE.md needs a marked import of
// AGENTS.md. It doesn't when there is no CLAUDE.md (Claude Code then reads
// AGENTS.md itself), when CLAUDE.md is AGENTS.md (a link), or when CLAUDE.md
// imports it outside a Laneway block. A CLAUDE.md with a Laneway block keeps
// it up to date.
func needsClaudeImport(p sdk.Project) bool {
	data, err := p.ReadFile(ClaudeFile)
	if err != nil {
		return false
	}
	if sameFile(p, ClaudeFile, AgentsFile) {
		return false
	}
	return strings.Contains(string(data), "<!-- laneway:begin") || !importsAgents.Match(data)
}

// sameFile reports whether a and b in the Project are one file.
func sameFile(p sdk.Project, a, b string) bool {
	ia, err := os.Stat(filepath.Join(p.Dir, a))
	if err != nil {
		return false
	}
	ib, err := os.Stat(filepath.Join(p.Dir, b))
	return err == nil && os.SameFile(ia, ib)
}

// BoostGuideline is where a Project using Laravel Boost gets the agent
// instructions: Boost writes the guideline files of the agents it's set up
// for (AGENTS.md, CLAUDE.md…) itself, inside its own block, and includes
// every file in .ai/guidelines when it does.
const BoostGuideline = ".ai/guidelines/laneway.md"

// UsesBoost reports whether the Project requires Laravel Boost. It reads
// composer.json, which is committed, rather than boost.json, which Boost
// suggests ignoring.
func UsesBoost(p sdk.Project) bool {
	data, err := p.ReadFile("composer.json")
	if err != nil {
		return false
	}
	var composer struct {
		Require    map[string]json.RawMessage `json:"require"`
		RequireDev map[string]json.RawMessage `json:"require-dev"`
	}
	if json.Unmarshal(data, &composer) != nil {
		return false
	}
	_, req := composer.Require["laravel/boost"]
	_, dev := composer.RequireDev["laravel/boost"]
	return req || dev
}

// BoostEffects put the agent instructions in Boost's custom guidelines, and
// take out the blocks an earlier run put in AGENTS.md and CLAUDE.md: Boost
// adds the guideline to those files, so they would say it twice, and
// CLAUDE.md importing AGENTS.md would repeat all of Boost's guidelines.
func BoostEffects(p sdk.Project) []sdk.Effect {
	guideline := sdk.Markdown(BoostGuideline, Block)
	guideline.Warning = fmt.Sprintf("this Project uses Laravel Boost, so the agent instructions are in %s; "+
		"run `php artisan boost:update` to add them to the guideline files of the agents Boost is set up for", BoostGuideline)
	effects := []sdk.Effect{
		sdk.File(guideline),
		sdk.File(sdk.FileBlock{Path: AgentsFile, RemoveMarkdown: true}),
	}
	if !sameFile(p, ClaudeFile, AgentsFile) {
		effects = append(effects, sdk.File(sdk.FileBlock{Path: ClaudeFile, RemoveMarkdown: true}))
	}
	return effects
}
