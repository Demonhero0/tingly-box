# Tingly-Project

A local replica of Claude's redesigned Projects: one conversation with a
coordinator that runs parallel Claude Code threads in git worktrees, on top of
Tingly Box. Same stack and conventions as tingly-box.

Research lives in `.research/`, design in `.design/`.

# Guideline

## UX-First (highest priority)

The product comes first. When UX conflicts with implementation convenience, push
UX further. **Before building or changing a feature, read
`.design/ux-principles.md`** (shared with tingly-box) and check the change
against its checklist.

**Before changing project behaviour, read `.research/claude-projects.md`
(what Claude's Projects are and how people use them) and `.design/projects.md`
(what we replicate, the MVP cut, and the architecture).** Keep parity with the
official experience where the design marks it in scope, and keep vocabulary
fixed: Project, Conversation, Coordinator, Thread, Overview, Library, Memory,
Instructions, Goal.

## Backend

- HTTP modules live in `internal/server/module/*` and implement `module.Module`.
  `internal/server/server_modules.go` is the only place modules are wired; the
  server and OpenAPI generation both use it.
- Errors use one shape, `{"error": "..."}`, via `module/apierr`.
- Metadata edits (rename, star, move) must not bump a conversation's
  `updated_at`; use the `*Meta` / `Set*` store helpers.
- LLM access goes through `internal/llm` only.

## Frontend

- Use MUI for components. Every page in `src/routes/appRoutes.tsx` is `lazy()`.
- Icons come from `@/components/icons` (Tabler via `tablerMui()`); do not import
  `@mui/icons-material` directly.
- Toasts: `useNotify()`. Copy feedback: `useCopyFeedback()`.
- Every user-facing string goes through i18n, in both `en.ts` and `zh.ts`
  (`zh.ts` is type-checked against `en.ts`).
- `localStorage` only for per-browser conveniences, via `utils/storage`.

# Cross frontend and backend

`openapi.json` and `frontend/src/client/schema.d.ts` are generated — never
hand-edit them. After changing API models or routes:

```bash
task codegen   # go run ./cmd/tingly-project swagger + pnpm gen:api
```

# Verify

```bash
task test
```
