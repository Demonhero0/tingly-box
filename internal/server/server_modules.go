package server

import (
	"github.com/tingly-dev/tingly-project/internal/db"
	"github.com/tingly-dev/tingly-project/internal/memory"
	"github.com/tingly-dev/tingly-project/internal/server/module"
	"github.com/tingly-dev/tingly-project/internal/server/module/conversation"
	"github.com/tingly-dev/tingly-project/internal/server/module/project"
	"github.com/tingly-dev/tingly-project/internal/server/module/settings"
)

// apiModules is the only place that decides which modules exist and how they
// are wired. The running server and OpenAPI generation both call it, so the
// spec can never drift from what is served.
func apiModules(store *db.Store) []module.Module {
	settingsSvc := settings.NewService(store)
	mem := memory.NewUpdater(store, settingsSvc)
	return []module.Module{
		project.NewHandler(store, mem),
		conversation.NewHandler(store, settingsSvc, mem),
		settings.NewHandler(settingsSvc),
		healthModule{},
	}
}
