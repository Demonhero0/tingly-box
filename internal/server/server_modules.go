package server

import (
	"github.com/tingly-dev/tingly-project/internal/db"
	"github.com/tingly-dev/tingly-project/internal/project"
	"github.com/tingly-dev/tingly-project/internal/server/module"
	"github.com/tingly-dev/tingly-project/internal/server/module/conversation"
	projectmod "github.com/tingly-dev/tingly-project/internal/server/module/project"
	"github.com/tingly-dev/tingly-project/internal/server/module/settings"
	"github.com/tingly-dev/tingly-project/internal/server/module/thread"
)

// Deps are what the modules are built from.
type Deps struct {
	Store  *db.Store
	Layout project.Layout
	// Runner runs thread turns; nil uses the Claude Code CLI.
	Runner project.Runner
}

// build is the only place that decides which modules exist and how they are
// wired. The running server and OpenAPI generation both call it, so the spec
// can never drift from what is served.
func build(d Deps) ([]module.Module, *project.Service) {
	settingsSvc := settings.NewService(d.Store)
	runner := d.Runner
	if runner == nil {
		runner = project.NewClaudeRunner()
	}
	svc := project.NewService(d.Store, d.Layout, runner, settingsSvc, settingsSvc)
	return []module.Module{
		projectmod.NewHandler(svc),
		thread.NewHandler(svc),
		conversation.NewHandler(d.Store, settingsSvc),
		settings.NewHandler(settingsSvc),
		healthModule{},
	}, svc
}
