# Projects — what we replicate from claude.ai

> **Scope note (2026-10-10):** this document covers only the *classic* chat
> Projects (folder model). Claude's Projects are being redesigned into a
> coordinator conversation plus parallel agent threads, and the classic
> version will be upgraded into it. See
> [`claude-projects-research.md`](claude-projects-research.md) for the full
> research and the gap analysis (§9) before extending this design.

tingly-project is a local, single-user replica of **claude.ai Projects**, built on
the same stack and conventions as tingly-box, with tingly-box as the default
model gateway.

## 1. Reference behaviour (claude.ai, researched 2026-10)

Sources: [What are projects?](https://support.claude.com/en/articles/9517075-what-are-projects),
[How can I create and manage projects?](https://support.claude.com/en/articles/9519177-how-can-i-create-and-manage-projects),
[Collaborate with Claude on Projects](https://www.anthropic.com/news/projects).

| Area | claude.ai behaviour |
|---|---|
| Project | Self-contained workspace with its own chat history and knowledge base. Created from Projects page → "+ New Project" with **name + description**. Claude **cannot see** name/description. |
| Instructions | "Set project instructions" — applied to every chat in the project (tone, role, perspective). |
| Knowledge | Right-hand panel on the project page; "+" adds documents, text, code. Used as context for every chat in the project. |
| Large knowledge | When knowledge nears the context window, **RAG mode turns on automatically** (paid plans), expanding capacity up to ~10×. |
| Context isolation | Chats in a project do **not** see each other, except through knowledge and memory. |
| Memory | Each project has its **own memory**, separate from non-project chats. A chat can opt out ("Memory" off in "+" menu) **before its first message**; it then neither uses nor feeds project memory. |
| Chats | Started from the project page. Standalone chats can be **added/moved to a project** ("Move chat" modal with search) and removed from it. |
| Star | From the card menu or the star in the project header; starred projects show in the left sidebar. |
| Archive | Menu → archive. Archived projects sit in an archived tab, chats stay accessible, **cannot be deleted until unarchived**. |
| Delete | Menu → "Delete" → confirm "Yes, delete". |
| Sharing | Team/Enterprise only: view/edit permissions, org visibility, "Shared with you" tab. |
| New beta | Project = one conversation fanning out cloud threads (Claude Code first). |

## 2. Scope

**In:** projects (name, description, star, archive, delete-guard), instructions,
knowledge (file upload + pasted text, view/edit/delete), automatic retrieval mode
for large knowledge, project memory (auto-updated, viewable, editable, per-chat
opt-out), project chats + standalone chats, move chat in/out of projects,
streaming chat with stop/regenerate/copy, auto titles, search, model picker,
model-connection settings with a real-path test.

**Out (for now):** sharing/permissions/org visibility (single-user local app),
the new "threads" beta, PDF/image extraction (text files only; PDFs are rejected
with a hint to paste their text), artifacts, web search, file attachments in a
single chat message.

## 3. How a project chat request is built

```
system:
  <documents>            ← knowledge: all files inline, or retrieved excerpts
  <project_memory>       ← unless the chat has memory off
  <project_instructions>
messages: the chat's own history only
```

Name and description are never sent — same as claude.ai.

### Knowledge: inline vs retrieval (no toggle)

- Total knowledge ≤ `InlineBudget` → every file is inlined in full.
- Above it, **retrieval mode** switches on automatically: files are split into
  ~800-token chunks, scored (BM25) against the latest user turns, and the top
  chunks up to `RetrievalBudget` are sent, grouped per source file.
- The capacity bar shows the concrete numbers (`used / inline budget` tokens) and
  a "Retrieval mode" label once it is active — principle 5 (concrete values) and
  6 (smart default instead of a switch).

### Memory

- Stored on the project as plain text, shown in the project panel, editable.
- After a project chat turn completes (memory on), a background job — debounced
  per project — asks the model to fold recent memory-enabled chats into the
  existing memory. "Update now" runs it on demand.

## 4. Stack (mirrors tingly-box)

| Layer | Choice | tingly-box counterpart |
|---|---|---|
| HTTP | gin + `github.com/tingly-dev/tingly-box/swagger` route groups | same |
| Modules | `internal/server/module/*` implementing `module.Module` | `.design/backend-organization.md` |
| Errors | one shape `{"error": "..."}` (`apierr`) | tingly-box keeps three for compat; we start with one |
| DB | gorm + sqlite | same |
| LLM | official `anthropic-sdk-go` / `openai-go` | `libs/` forks of the same SDKs |
| Gateway | default base URL `http://localhost:12580/tingly/anthropic` | tingly-box scenario routes |
| Frontend | React + MUI + Vite, `openapi-typescript` + `openapi-fetch`, Tabler icons via `tablerMui`, i18n en/zh | same |
| Codegen | `task codegen` = `swagger` (openapi.json) + `pnpm gen:api` | same |
