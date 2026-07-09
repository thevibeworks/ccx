// Package skills embeds the agent skills that ship with ccx so the
// binary can install skill copies matching its own CLI surface.
// Skills version with the repo, binaries version with tags; without
// this, a released binary and the skills beside it drift apart and
// agents drive flags that don't exist (issue #19).
package skills

import "embed"

//go:embed */SKILL.md
var FS embed.FS
