# tingly-project

A local replica of Claude's redesigned **Projects**: one conversation where a
coordinator turns the work you send into parallel **threads** — Claude Code
sessions, each on its own git worktree branch — and tells you what is ready for
review and what is waiting on you. Built on [Tingly Box](https://github.com/tingly-dev/tingly-box).

- **One place to send work.** Paste a task list, a bug report or a stack trace
  into the project conversation. The coordinator answers quick questions,
  starts a thread per task, or hands follow-ups to the thread already working
  in that area.
- **Threads are real Claude Code sessions** in their own worktree and branch.
  Permission requests and questions wait for you inside the thread.
- **Overview** groups threads by state — Waiting on you, Ready for review,
  Working, Idle, Resolved. Review a thread's diff and merge it into its base
  branch from there.
- **Standing context:** a one-line goal, project instructions, memory files
  (`MEMORY.md` + notes Claude writes and you can edit) and a Library of files.
- **Local and multi-model:** threads run on your machine; coordinator and
  threads can use different models through Tingly Box.

Research: [`.research/claude-projects.md`](.research/claude-projects.md) ·
Design: [`.design/projects.md`](.design/projects.md)

## Quick start

Requires `git` and the [Claude Code CLI](https://docs.claude.com/en/docs/claude-code) (`claude` on `PATH`).

```bash
task build                 # builds the web UI and the binary
./bin/tingly-project       # http://127.0.0.1:12590
```

By default the coordinator and threads go through a local Tingly Box at
`http://localhost:12580/tingly/anthropic`. Open **Settings** to paste a Tingly
Box model token, pick a default model and send a test message — or point it at
any Anthropic-compatible endpoint (OpenAI-style endpoints work for the
coordinator and chats; threads then use Claude Code's own login).

Data lives in `~/.tingly-project` (override with `--data-dir` or
`$TINGLY_PROJECT_HOME`); each project's memory, library and thread worktrees
are under `projects/<id>/`. The server binds to `127.0.0.1` and has no login.

## Development

```bash
task dev:backend    # API on :12590
task dev:frontend   # Vite on :3010, proxies /api
task test           # go vet + go test + typecheck + lint + vitest
task codegen        # after changing API models/routes
```

| Layer | Stack |
|---|---|
| Backend | Go, gin, [`tingly-box/swagger`](https://github.com/tingly-dev/tingly-box/tree/main/swagger) route groups, gorm + SQLite, [`tingly-box/agentboot`](https://github.com/tingly-dev/tingly-box/tree/main/agentboot) for Claude Code, official Anthropic / OpenAI Go SDKs |
| Frontend | React 19, MUI, Vite, `openapi-typescript` + `openapi-fetch`, Tabler icons, i18next (en / zh) |
