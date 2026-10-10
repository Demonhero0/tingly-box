# tingly-project

Claude-style **Projects** — shared instructions, knowledge, memory and chats —
running locally on top of [Tingly Box](https://github.com/tingly-dev/tingly-box).

A project keeps related chats together. Every chat in it starts with:

- **Instructions** — how the model should respond (tone, role, format).
- **Knowledge** — documents, code and notes you add. Sent in full up to 100k
  tokens; beyond that, **retrieval mode** switches on automatically and sends the
  most relevant excerpts (up to 1M tokens of knowledge).
- **Memory** — a model-written summary of what your chats in the project
  established, updated automatically and editable. A chat can opt out before
  its first message.

Chats stay isolated from each other; they share context only through knowledge
and memory. The project name and description are for you and are never sent to
the model. See [`.design/projects.md`](.design/projects.md) for the reference
behaviour and design decisions.

## Quick start

```bash
task build                 # builds the web UI and the binary
./bin/tingly-project       # http://127.0.0.1:12590
```

By default chats go to a local Tingly Box at
`http://localhost:12580/tingly/anthropic`. Open **Settings** to paste a Tingly
Box model token, pick a default model and send a test message — or point it at
any Anthropic- or OpenAI-compatible endpoint.

Data lives in `~/.tingly-project` (override with `--data-dir` or
`$TINGLY_PROJECT_HOME`). The server binds to `127.0.0.1` and has no login; use
`--host 0.0.0.0` only on a trusted network.

## Development

```bash
task dev:backend    # API on :12590
task dev:frontend   # Vite on :3010, proxies /api
task test           # go vet + go test + typecheck + lint + vitest
task codegen        # after changing API models/routes
```

| Layer | Stack |
|---|---|
| Backend | Go, gin, [`tingly-box/swagger`](https://github.com/tingly-dev/tingly-box/tree/main/swagger) route groups, gorm + SQLite, official Anthropic / OpenAI Go SDKs |
| Frontend | React 19, MUI, Vite, `openapi-typescript` + `openapi-fetch`, Tabler icons, i18next (en / zh) |
