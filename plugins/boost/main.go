// Command boost is the boost Plugin: the Laravel Boost Generator, run by
// `lane generate boost`. It gives Laravel Boost the agent instructions as a
// custom guideline, and takes out the blocks an earlier Harness Generator
// run put in AGENTS.md and CLAUDE.md (see agents.BoostEffects).
package main

import (
	"github.com/lanewayhq/plugins/internal/agents"
	"github.com/lanewayhq/plugins/sdk"
)

func main() {
	sdk.Run(sdk.Plugin{Generators: map[string]sdk.Generator{"boost": generate}})
}

// generate is the Laravel Boost Generator: it gives Boost the agent
// instructions (see agents.BoostEffects).
func generate(p sdk.Project) (sdk.GeneratorResult, error) {
	return sdk.GeneratorResult{Effects: agents.BoostEffects(p)}, nil
}
