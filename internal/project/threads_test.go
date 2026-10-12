package project

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/agentboot"
	"github.com/tingly-dev/tingly-project/internal/db"
)

// scriptRunner runs each turn through a test-provided function.
type scriptRunner struct {
	mu    sync.Mutex
	turns []TurnSpec
	fn    func(ctx context.Context, spec TurnSpec, p agentboot.Prompter, sink func(RunnerEvent)) (TurnResult, error)
}

func (r *scriptRunner) RunTurn(ctx context.Context, spec TurnSpec, p agentboot.Prompter, sink func(RunnerEvent)) (TurnResult, error) {
	r.mu.Lock()
	r.turns = append(r.turns, spec)
	r.mu.Unlock()
	return r.fn(ctx, spec, p, sink)
}

func (r *scriptRunner) specs() []TurnSpec {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]TurnSpec(nil), r.turns...)
}

type staticEnv struct{}

func (staticEnv) ThreadEnv(context.Context) ([]string, string) {
	return []string{"ANTHROPIC_BASE_URL=http://gw"}, "default-model"
}

func run(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, string(out))
	return strings.TrimSpace(string(out))
}

func newRepo(t *testing.T) string {
	dir := t.TempDir()
	run(t, dir, "git", "init", "-q", "-b", "main")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "README.md"), []byte("hi\n"), 0o644))
	run(t, dir, "git", "add", ".")
	run(t, dir, "git", "commit", "-qm", "init")
	return dir
}

type testFixture struct {
	store   *db.Store
	threads *Threads
	runner  *scriptRunner
	project *db.Project
	repo    string
	reports chan Report
}

func newFixture(t *testing.T, fn func(ctx context.Context, spec TurnSpec, p agentboot.Prompter, sink func(RunnerEvent)) (TurnResult, error)) *testFixture {
	store, err := db.Open(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { store.Close() })
	layout := Layout{Root: t.TempDir()}
	repo := newRepo(t)
	p := &db.Project{Name: "Demo", Goal: "Ship it", Instructions: "Be careful.", Repos: []db.Repo{{Path: repo, BaseBranch: "main"}}}
	require.NoError(t, store.CreateProject(p))
	require.NoError(t, layout.Ensure(p.ID))
	r := &scriptRunner{fn: fn}
	th := NewThreads(store, layout, NewHub(), r, staticEnv{})
	f := &testFixture{store: store, threads: th, runner: r, project: p, repo: repo, reports: make(chan Report, 10)}
	th.OnReport = func(rep Report) { f.reports <- rep }
	return f
}

func (f *testFixture) report(t *testing.T) Report {
	t.Helper()
	select {
	case r := <-f.reports:
		return r
	case <-time.After(5 * time.Second):
		t.Fatal("no report")
		return Report{}
	}
}

func waitStatus(t *testing.T, store *db.Store, id, status string) *db.Thread {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		th, err := store.GetThread(id)
		require.NoError(t, err)
		if th.Status == status {
			return th
		}
		time.Sleep(10 * time.Millisecond)
	}
	th, _ := store.GetThread(id)
	t.Fatalf("thread status %q, want %q (%s)", th.Status, status, th.WaitReason)
	return nil
}

func TestThreadCommitsBecomeReadyAndMerge(t *testing.T) {
	var f *testFixture
	f = newFixture(t, func(ctx context.Context, spec TurnSpec, p agentboot.Prompter, sink func(RunnerEvent)) (TurnResult, error) {
		sink(RunnerEvent{Kind: db.EvAssistant, Content: "Adding the file"})
		require.NoError(t, os.WriteFile(filepath.Join(spec.Dir, "feature.txt"), []byte("x\n"), 0o644))
		run(t, spec.Dir, "git", "add", ".")
		run(t, spec.Dir, "git", "commit", "-qm", "feature")
		return TurnResult{Text: "Added feature.txt and committed it.", InputTokens: 10, OutputTokens: 5}, nil
	})
	th, err := f.threads.Start(context.Background(), f.project, StartSpec{Title: "Add feature", Task: "add feature.txt", Origin: "coordinator"})
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(th.Branch, "tp/add-feature-"))

	rep := f.report(t)
	assert.Equal(t, ReportFinished, rep.Kind)
	got := waitStatus(t, f.store, th.ID, db.ThreadReady)
	assert.Equal(t, 1, got.CommitsAhead)
	assert.Equal(t, 1, got.FilesChanged)
	assert.EqualValues(t, 10, got.InputTokens)
	assert.Equal(t, "default-model", got.Model)

	spec := f.runner.specs()[0]
	assert.False(t, spec.Resume)
	assert.Equal(t, DefaultPermissionMode, spec.PermissionMode)
	assert.Contains(t, spec.AppendSystemPrompt, "Be careful.")
	assert.Contains(t, spec.AppendSystemPrompt, th.Branch)
	assert.Contains(t, spec.Env, "ANTHROPIC_BASE_URL=http://gw")

	d, err := f.threads.Diff(context.Background(), th.ID)
	require.NoError(t, err)
	assert.Contains(t, d.Patch, "feature.txt")

	merged, err := f.threads.Merge(context.Background(), th.ID)
	require.NoError(t, err)
	assert.Equal(t, db.ThreadResolved, merged.Status)
	assert.FileExists(t, filepath.Join(f.repo, "feature.txt"))
}

func TestMergeRefusesDirtyCheckout(t *testing.T) {
	f := newFixture(t, func(ctx context.Context, spec TurnSpec, p agentboot.Prompter, sink func(RunnerEvent)) (TurnResult, error) {
		require.NoError(t, os.WriteFile(filepath.Join(spec.Dir, "a.txt"), []byte("a\n"), 0o644))
		run(t, spec.Dir, "git", "add", ".")
		run(t, spec.Dir, "git", "commit", "-qm", "a")
		return TurnResult{Text: "done"}, nil
	})
	th, err := f.threads.Start(context.Background(), f.project, StartSpec{Task: "a"})
	require.NoError(t, err)
	f.report(t)
	waitStatus(t, f.store, th.ID, db.ThreadReady)
	require.NoError(t, os.WriteFile(filepath.Join(f.repo, "README.md"), []byte("local edit\n"), 0o644))
	_, err = f.threads.Merge(context.Background(), th.ID)
	var blocked *MergeBlockedError
	require.ErrorAs(t, err, &blocked)
	assert.Contains(t, blocked.Command, "merge --no-ff")
}

func TestApprovalWaitsOnUser(t *testing.T) {
	f := newFixture(t, func(ctx context.Context, spec TurnSpec, p agentboot.Prompter, sink func(RunnerEvent)) (TurnResult, error) {
		resp, err := p.OnApproval(ctx, agentboot.ApprovalRequestEvent{ID: "req-1", ToolName: "Bash", Input: map[string]any{"command": "make test"}})
		if err != nil {
			return TurnResult{}, err
		}
		if !resp.Approved {
			return TurnResult{Text: "Not allowed to run tests."}, nil
		}
		// A second Bash call is allowed for the rest of the thread.
		resp2, _ := p.OnApproval(ctx, agentboot.ApprovalRequestEvent{ID: "req-2", ToolName: "Bash", Input: map[string]any{"command": "make lint"}})
		return TurnResult{Text: map[bool]string{true: "Tests and lint pass.", false: "lint blocked"}[resp2.Approved]}, nil
	})
	th, err := f.threads.Start(context.Background(), f.project, StartSpec{Task: "run tests"})
	require.NoError(t, err)
	got := waitStatus(t, f.store, th.ID, db.ThreadWaiting)
	assert.Equal(t, "Approve: Run `make test`", got.WaitReason)

	require.NoError(t, f.threads.Respond(th.ID, "req-1", Answer{Allow: true, Scope: "thread"}))
	rep := f.report(t)
	assert.Equal(t, "Tests and lint pass.", rep.Text)
	waitStatus(t, f.store, th.ID, db.ThreadIdle)

	events, _ := f.store.ListThreadEvents(th.ID)
	var approvals int
	for _, e := range events {
		if e.Kind == db.EvApproval {
			approvals++
			assert.Contains(t, e.Payload, `"state":"allowed"`)
		}
	}
	assert.Equal(t, 1, approvals, "the second call was allowed by the thread-wide answer")
}

func TestNeedsInputAndFollowUpResumes(t *testing.T) {
	turn := 0
	f := newFixture(t, func(ctx context.Context, spec TurnSpec, p agentboot.Prompter, sink func(RunnerEvent)) (TurnResult, error) {
		turn++
		if turn == 1 {
			return TurnResult{Text: "Two options exist.\nNEEDS_INPUT: Should I use Postgres or SQLite?"}, nil
		}
		return TurnResult{Text: "Used " + spec.Prompt}, nil
	})
	th, err := f.threads.Start(context.Background(), f.project, StartSpec{Task: "pick a db"})
	require.NoError(t, err)
	rep := f.report(t)
	assert.Equal(t, ReportNeedsInput, rep.Kind)
	got := waitStatus(t, f.store, th.ID, db.ThreadWaiting)
	assert.Equal(t, "Should I use Postgres or SQLite?", got.WaitReason)

	_, err = f.threads.Send(context.Background(), th.ID, "SQLite", "user")
	require.NoError(t, err)
	rep = f.report(t)
	assert.Equal(t, "Used SQLite", rep.Text)
	specs := f.runner.specs()
	require.Len(t, specs, 2)
	assert.True(t, specs[1].Resume)
	assert.Equal(t, specs[0].SessionID, specs[1].SessionID)
}

func TestStopAndPausedProject(t *testing.T) {
	started := make(chan struct{})
	f := newFixture(t, func(ctx context.Context, spec TurnSpec, p agentboot.Prompter, sink func(RunnerEvent)) (TurnResult, error) {
		close(started)
		<-ctx.Done()
		return TurnResult{}, ctx.Err()
	})
	th, err := f.threads.Start(context.Background(), f.project, StartSpec{Task: "long"})
	require.NoError(t, err)
	<-started
	f.threads.Stop(th.ID)
	waitStatus(t, f.store, th.ID, db.ThreadIdle)
	select {
	case r := <-f.reports:
		t.Fatalf("a stopped turn must not report, got %+v", r)
	case <-time.After(100 * time.Millisecond):
	}

	f.project.Paused = true
	require.NoError(t, f.store.SaveProject(f.project))
	_, err = f.threads.Send(context.Background(), th.ID, "go on", "user")
	assert.ErrorIs(t, err, ErrPaused)
}

func TestMemory(t *testing.T) {
	l := Layout{Root: t.TempDir()}
	require.NoError(t, l.Ensure("p"))
	name, err := l.Remember("p", "Release date", "Release moved to Friday.", "release is Friday")
	require.NoError(t, err)
	assert.Equal(t, "release-date.md", name)
	_, err = l.Remember("p", "Release date", "Release moved to Monday.", "release is Monday")
	require.NoError(t, err)
	idx := l.memoryIndexText("p")
	assert.Equal(t, 1, strings.Count(idx, "release-date.md"), idx)
	assert.Contains(t, idx, "release is Monday")
	require.NoError(t, l.DeleteMemory("p", name))
	assert.NotContains(t, l.memoryIndexText("p"), "release-date.md")
	assert.Error(t, l.DeleteMemory("p", MemoryIndex))
	_, err = l.ReadMemory("p", "../etc.md")
	assert.Error(t, err)
}
