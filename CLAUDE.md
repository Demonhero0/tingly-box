# Tingly-Project

Claude-style Projects (instructions, knowledge, memory, chats) on top of Tingly
Box. Same stack and conventions as tingly-box.

# Guideline

## UX-First (highest priority)

The product comes first. When UX conflicts with implementation convenience, push
UX further. **Before building or changing a feature, read
`.design/ux-principles.md`** (shared with tingly-box) and check the change
against its checklist.

**Before changing project behaviour, read `.design/projects.md`.** It records
what claude.ai Projects does, what we replicate, and what is deliberately out of
scope (e.g. sharing). Keep claude.ai parity where it is listed as in scope —
e.g. name/description are never sent to the model; archived projects cannot be
deleted; memory can only be turned off before a chat's first message.

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
