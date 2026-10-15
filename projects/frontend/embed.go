// Package frontend embeds the built web UI (frontend/dist) into the binary.
// Build it with `task frontend` (or `pnpm build` here) before `go build`.
package frontend

import "embed"

// Dist is the built web UI. It holds only .gitkeep until the UI is built.
//
//go:embed all:dist
var Dist embed.FS
