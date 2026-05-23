# Crowbar Web UI — Production Readiness Spec

**Date:** 2026-05-23
**Status:** Approved
**Companion to:** `2026-05-23-web-ui-design.md`

---

## Overview

This spec covers the five waves of work needed to close the gap between the current mock UI and a production-ready frontend. The backend is being built in parallel; where backend contracts don't exist yet, we mock at the seam (the API layer in `lib/api.ts`) so the UI is backend-ready without blocking.

**Component rule:** 100% shadcn/ui + Vercel AI Elements (`npx ai-elements`). No bespoke UI primitives.

---

## Wave 1 — Sidebar State Bugs

Two bugs found in live testing that affect every other surface.

### Bug 1: New chat renders previous chat's messages

**Root cause:** `$chatId.tsx` reads messages from `lib/mock/conversations.ts`. When the ID is unknown (new chat), the mock returns the first chat's data instead of an empty array.

**Fix:** Guard in `lib/mock/conversations.ts` — return `[]` for unknown chat IDs. New chat route gets an empty message list and shows the empty state.

### Bug 2: New workspace not added to sidebar

**Root cause:** `WorkspaceCreationForm` navigates to the new workspace URL but never writes the new workspace into the in-memory store that `__root.tsx` reads from.

**Fix:** After `postWorkspace()` resolves, invalidate the workspace query so `__root.tsx` re-fetches and the new workspace row appears in the sidebar. In mock mode, write the new workspace directly into `lib/mock/workspaces.ts`'s in-memory Map before navigating.

### Repo collapse/expand

**Current state:** Chevron is rendered in `RepoRow` but wired to nothing.

**Fix:** `__root.tsx` holds a `Set<string>` of collapsed repo IDs in `useState`. `RepoRow` receives `collapsed: boolean` and `onToggle: () => void`. Workspace rows under a collapsed repo are conditionally rendered. Collapsed state is not persisted (resets on reload — acceptable for now).

---

## Wave 2 — Project Shell

### Data model

```ts
interface Project {
  id: string
  name: string       // folder name or user-defined
  path: string       // absolute local path
  repos: Repo[]      // auto-detected git repos within the path
  lastActivity: Date
}
```

In mock mode: two hardcoded projects (`rabbyte` at a fake path, `personal` at another). The active project ID is stored in `localStorage` and read at app boot.

### Routes

| Route | Component | Purpose |
|---|---|---|
| `/projects` | `ProjectListPage` | Grid of imported projects + import CTA |
| `/projects/new` | `ProjectListPage` (modal open) | Import flow entry point |
| `/` | redirect | → `/projects` if no active project; → active project workspace otherwise |

### Project list page (`/projects`)

shadcn `Card` grid (2-col on wide screens, 1-col on narrow). Each card:
- Project name (`font-semibold`)
- Local path (`text-muted-foreground font-mono text-xs truncate`)
- Repo count badge
- Last activity string
- Clicking the card sets it as active project and navigates to its workspace

**Empty state:** Centered illustration + "No projects yet" heading + "Import project" `Button`.

### Import modal

shadcn `Dialog`. Single step:
1. **Folder picker** — a hidden `<input type="file" webkitdirectory>` triggered by a shadcn `Button`. Shows the selected folder path in a read-only `Input` once picked. Label: "Project folder".
2. **Project name** — `Input`, auto-populated from the selected folder name, editable.
3. **Import button** — disabled until a folder has been selected. On submit: calls `postProject({ path, name })`, closes modal, adds project card.

The OS native file picker opens via `.click()` on the hidden input. `webkitdirectory` restricts selection to folders only. The resolved absolute path is read from `file.webkitRelativePath` or `file.path` (Electron/Tauri) depending on the runtime.

### Project switcher

`SidebarHeader` "Projects / Rabbyte" breadcrumb:
- "Projects" → `Link` to `/projects`
- Project name → shadcn `DropdownMenu` listing all imported projects + "Manage projects" footer link to `/projects`

Selecting a project from the dropdown sets it active in localStorage and navigates to that project's root.

---

## Wave 3 — Chat Input Bar

Replaces the current bespoke `ChatInput.tsx` entirely. Built from AI Elements `PromptInput` as the outer shell, with three additions in the footer row.

### Layout

```
┌──────────────────────────────────────────────────────────────┐
│  Message… (brainstorming)                                    │
│                                                              │
├──────────────────────────────────────────────────────────────┤
│  [📎]  [✦ Sonnet 4.6 ▼]  [Low · Mid · High · Max]  ·· 4%  [→]│
└──────────────────────────────────────────────────────────────┘
```

### Model selector

AI Elements `ModelSelector` component. Options:

| Label | Value |
|---|---|
| Haiku 4.5 | `claude-haiku-4-5-20251001` |
| Sonnet 4.6 | `claude-sonnet-4-6` |
| Opus 4.7 | `claude-opus-4-7` |

Selected model stored in `localStorage` key `crowbar.model`. Default: `claude-sonnet-4-6`.

### Effort level

shadcn `ToggleGroup` with four values: `low`, `medium`, `high`, `max`. Renders as a compact pill row in the footer. Stored in `localStorage` key `crowbar.effort`. Default: `medium`.

Maps to Anthropic's `thinking` budget tokens when the backend is ready:
- `low` → disabled
- `medium` → 2 000 tokens
- `high` → 8 000 tokens
- `max` → 32 000 tokens (or model cap)

### Attachments

AI Elements `Attachments` component. Trigger: paperclip icon button left of the model selector.

Supported types for now: any file (backend decides what to do with it). Attachments are held in local state as `File[]` and cleared after send. Each attachment shows a thumbnail or file-type icon in a row above the input.

### Token counter

Character count of the current message ÷ 4 (rough token estimate). Displayed as `·· N%` next to the send button using shadcn `Progress` (thin, 3px). Updates on every keystroke. Does not account for attachments or conversation history (accurate-enough for mock phase).

### State

`ChatInput` is a controlled component. Parent (`$step.tsx`) owns:
- `message: string`
- `attachments: File[]`
- `model: string` (from localStorage)
- `effort: string` (from localStorage)
- `onSend(payload): void`

---

## Wave 4 — Chat Experience

### Streaming render

Replace the 1.5s fake delay with a mock `ReadableStream` that chunks the response string at ~30ms intervals (simulates token streaming). This makes the UI streaming-ready without the backend.

AI Elements components used:
- `Conversation` — outer scroll container, auto-scrolls to bottom on new content
- `Message` — individual message bubble (handles user vs. assistant variants)
- `Shimmer` — shown from send until the first token arrives

When the backend is ready: swap the mock `ReadableStream` for the real SSE stream from `useEvents`. No component changes needed.

### Tool call display

Replace the plain "4 tool calls · 18.3s" `Separator` text with AI Elements `Tool` component.

Each tool call:
- Collapsed by default: shows tool name + duration
- Expandable: shows input JSON and output JSON in shadcn `Code` blocks

The mock data in `lib/mock/conversations.ts` gains a `toolCalls` array per turn:

```ts
interface ToolCall {
  name: string
  durationMs: number
  input: Record<string, unknown>
  output: Record<string, unknown>
}
```

### Empty state for new chats / workspaces

When `messages.length === 0`:

```
         ✦
   Start a conversation
   Ask anything about this workspace
```

Centered vertically in the chat area. Uses `text-muted-foreground`. No buttons — user just types.

---

## Wave 5 — Polish

### Error boundaries

shadcn doesn't ship an error boundary; we write a single `ErrorBoundary` component using React's `componentDidCatch`.

Two placements:
1. **Root** — wraps `<Outlet />` in `__root.tsx`. Shows a full-page error state.
2. **Chat area** — wraps `<ChatView />` in `$step.tsx`. Shows an inline "Something went wrong" card with a retry button.

Error state card: shadcn `Card` with `bg-destructive/10 border-destructive/20`, error message in `text-destructive`, and a `Button variant="outline"` labelled "Try again" that calls `reset()`.

### Sidebar loading skeletons

While workspace/chat data loads (query `isPending`): render 3–4 `Shimmer` rows in place of the real sidebar rows. Same height and padding as real rows so the layout doesn't shift.

### Route: Projects breadcrumb

Wire `<Link to="/projects">Projects</Link>` in `SidebarHeader`. Currently inert.

---

## Mock → Real API Seam

All mock behaviour lives in `lib/api.ts` and `lib/mock/`. When the backend is ready, replace the mock functions in `lib/api.ts` with real `apiFetch()` calls. No component changes required.

| Mock function | Real endpoint |
|---|---|
| `fetchProjects()` | `GET /api/v0/projects` |
| `postProject(path, name)` | `POST /api/v0/projects` |
| `fetchWorkspace(wsId)` | `GET /api/v0/workspaces/:id` |
| `fetchConversation(wsId, step)` | `GET /api/v0/workspaces/:id/steps/:step/messages` |
| `postMessage(wsId, step, payload)` | `POST /api/v0/workspaces/:id/steps/:step/messages` (SSE stream) |

---

## Open Questions (deferred)

- Auth — hardcoded "MU" / Mateo is acceptable until the backend defines an auth contract.
- IDE surface (file explorer, editing, review phase) — explicitly out of scope for this spec.
- DiffView — out of scope.
