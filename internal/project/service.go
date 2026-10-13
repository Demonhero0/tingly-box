// Package project is the runtime behind Projects: a coordinator conversation
// per project that routes work to threads, each a Claude Code session on its
// own git worktree branch. See .design/projects.md.
package project

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/tingly-dev/tingly-project/internal/db"
	"github.com/tingly-dev/tingly-project/llm"
)

// Host is what the runtime needs from whoever runs it: a model client for
// the coordinator, the models the project settings offer, and the
// environment threads run Claude Code in. The standalone app implements it
// with its own connection settings; an embedding host (Tingly Box) with its
// gateway.
type Host interface {
	LLMSource
	ThreadEnv
	Models(ctx context.Context) ([]llm.Model, error)
}

// Service bundles the project runtime for the HTTP modules.
type Service struct {
	Store       *db.Store
	Layout      Layout
	Hub         *Hub
	Threads     *Threads
	Coordinator *Coordinator
	Host        Host
}

// NewService wires the runtime. Call Start once the server is up.
func NewService(store *db.Store, layout Layout, agents Agents, host Host) *Service {
	hub := NewHub()
	threads := NewThreads(store, layout, hub, agents, host)
	return &Service{
		Store: store, Layout: layout, Hub: hub, Threads: threads,
		Coordinator: NewCoordinator(store, layout, hub, threads, host),
		Host:        host,
	}
}

// Start recovers interrupted threads and runs housekeeping until ctx ends,
// then stops every thread's turn and process.
func (s *Service) Start(ctx context.Context) {
	s.Threads.Recover()
	go func() {
		tick := time.NewTicker(time.Hour)
		defer tick.Stop()
		for {
			s.Threads.AutoResolve(7 * 24 * time.Hour)
			select {
			case <-ctx.Done():
				shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				s.Threads.Shutdown(shutdown)
				cancel()
				return
			case <-tick.C:
			}
		}
	}()
}

// NewProjectSpec is the New project dialog.
type NewProjectSpec struct {
	Name  string
	Goal  string
	Repos []string
}

// ResolveRepos validates repository paths and records their current branch
// as the base threads branch from.
func ResolveRepos(ctx context.Context, paths []string) ([]db.Repo, error) {
	seen := map[string]bool{}
	repos := []db.Repo{}
	for _, raw := range paths {
		path := strings.TrimSpace(raw)
		if path == "" {
			continue
		}
		if strings.HasPrefix(path, "~/") {
			if home, err := os.UserHomeDir(); err == nil {
				path = filepath.Join(home, path[2:])
			}
		}
		info, err := InspectRepo(ctx, path)
		if err != nil {
			return nil, err
		}
		if seen[info.Root] {
			continue
		}
		seen[info.Root] = true
		repos = append(repos, db.Repo{Path: info.Root, BaseBranch: info.Branch})
	}
	return repos, nil
}

// Create makes a project and its directories.
func (s *Service) Create(ctx context.Context, spec NewProjectSpec) (*db.Project, error) {
	name := strings.TrimSpace(spec.Name)
	if name == "" {
		return nil, errors.New("name is required")
	}
	repos, err := ResolveRepos(ctx, spec.Repos)
	if err != nil {
		return nil, err
	}
	p := &db.Project{Name: name, Goal: strings.TrimSpace(spec.Goal), Repos: repos, ThreadPermissionMode: DefaultPermissionMode}
	if err := s.Store.CreateProject(p); err != nil {
		return nil, err
	}
	if err := s.Layout.Ensure(p.ID); err != nil {
		return nil, err
	}
	return p, nil
}

// Save persists a project change and tells open pages.
func (s *Service) Save(p *db.Project) error {
	if len([]rune(p.Instructions)) > MaxInstructions {
		return fmt.Errorf("instructions exceed %d characters", MaxInstructions)
	}
	if err := s.Store.SaveProject(p); err != nil {
		return err
	}
	s.Hub.Publish(Event{Type: EventProject, ProjectID: p.ID, Data: p})
	return nil
}

// SetPaused pauses (stopping every running thread) or resumes a project.
func (s *Service) SetPaused(p *db.Project, paused bool) error {
	p.Paused = paused
	if paused {
		s.Threads.StopProject(p.ID)
	}
	return s.Save(p)
}

// SetArchived archives (stopping its threads) or unarchives a project.
func (s *Service) SetArchived(p *db.Project, archived bool) error {
	if archived {
		now := time.Now()
		p.ArchivedAt = &now
		s.Threads.StopProject(p.ID)
	} else {
		p.ArchivedAt = nil
	}
	return s.Save(p)
}

// Delete removes a project, its threads' worktrees, memory and library.
// Branches the threads created are kept.
func (s *Service) Delete(ctx context.Context, p *db.Project) error {
	s.Threads.StopProject(p.ID)
	s.Threads.RemoveWorktrees(ctx, p.ID)
	if err := s.Store.DeleteProject(p.ID); err != nil {
		return err
	}
	return os.RemoveAll(s.Layout.Dir(p.ID))
}
