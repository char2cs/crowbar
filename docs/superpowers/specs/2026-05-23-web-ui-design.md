# Crowbar Web UI — Design Spec

**Date:** 2026-05-23
**Status:** Approved

---

## Overview

A desktop-first React web UI for Crowbar built on **shadcn/ui (Tailwind v4 canary)** and **Vercel AI SDK UI elements**. No custom colors are hardcoded — all color references use shadcn/ui CSS variable tokens (`bg-background`, `text-muted-foreground`, `border`, etc.). The layout is a two-panel shell: a resizable sidebar on the left and a main content area on the right.

---

## Tech Stack

- **React 19** + **Vite 6** + **TypeScript**
- **Tailwind CSS v4** via `@tailwindcss/vite`
- **shadcn/ui canary** — Tailwind v4-compatible, installed via `npx shadcn@canary`
- **Vercel AI SDK UI elements** (`@ai-sdk/react` + `ai`) for chat primitives
- **TanStack Router** (already wired) for routing
- **TanStack Query** (already wired) for API state

---

## Color & Theming

- Use shadcn/ui's default dark theme as the app baseline.
- All Tailwind classes reference semantic tokens only: `bg-background`, `bg-card`, `bg-muted`, `text-foreground`, `text-muted-foreground`, `text-primary`, `border`, `ring`, etc.
- No hardcoded color values anywhere in JSX or CSS.
- The single dark theme definition lives in `web/src/index.css` as shadcn/ui CSS variable overrides.

---

## Layout

### Shell (`AppShell`)

```
┌──────────────────┬──────────────────────────────────┐
│     Sidebar      │           Main panel              │
│  (resizable)     │                                   │
│  min: 180px      │                                   │
│  default: 256px  │                                   │
│  max: 400px      │                                   │
└──────────────────┴──────────────────────────────────┘
```

- Sidebar width is stored in `localStorage` and restored on mount.
- A drag handle on the right edge of the sidebar allows resizing (highlights on hover).
- The border between sidebar and main uses `border-border`.

---

## Sidebar

### Header

Fixed 48px height. Contains:
- **App icon** — 24px rounded square with `bg-primary` gradient.
- **Breadcrumb** — `Projects / {project.name}`. "Projects" is a muted link, project name is `font-semibold text-foreground`. Uses shadcn `Button variant="ghost"` for the clickable parts.
- **User avatar** — shadcn `Avatar` (24px) pinned to the right.

### Scroll area

shadcn `ScrollArea` wrapping all rows below the header.

### Row system

Every item in the sidebar — chat entries, repo headers, workspace entries, and new-item buttons — is a **single row component** with:
- **Height:** 36px (fixed via `h-9`)
- **Padding:** `px-2 mx-1.5`
- **Border radius:** `rounded-lg` (consistent everywhere)
- **Gap between rows:** `my-0.5`
- **Hover:** `hover:bg-accent`
- **Active:** `bg-accent` (workspace selected) or `bg-primary/10` (chat selected)

Row variants:

| Variant | Left element | Body | Right |
|---|---|---|---|
| `ChatRow` | shadcn `Avatar` (icon) | Title + `text-muted-foreground` | Age string + close `×` on hover |
| `RepoRow` | shadcn `Avatar` (letter) | Repo name `font-medium` | Collapse chevron |
| `WorkspaceRow` | Number `text-muted-foreground` | Branch name (mono) + stats + age subtitle | Close `×` on hover |
| `NewRow` | `+` icon | "New chat" / "New workspace" `text-muted-foreground` | — |

### Grouping

- Project-level chats appear at the top of the scroll area, followed by a `+ New chat` row.
- A shadcn `Separator` divides project chats from repositories.
- Each repo renders as a `RepoRow` (collapsible folder), followed by its `WorkspaceRow` children and a `+ New workspace` row.
- No section label text — structure is communicated by visual grouping alone.

---

## Main Panel

### Flow step tabs

shadcn `Tabs` component, full-width, 40px height. The four steps in order:

1. **Brainstorm** — chat interface (this spec)
2. **Spec** — chat interface
3. **Builder** — chat interface
4. **AI Review** — chat interface (future: diff/PR view)
5. **Human Review** — future: PR review interface

Each tab has a 6px status dot:
- `bg-green-500` — completed step
- `bg-primary` — active step
- `bg-muted` — pending step

Arrows (`›`) between tabs are `text-muted-foreground`, not interactive.

### Chat view (active for Brainstorm + Spec steps)

Vercel AI SDK `useChat` hook drives the message list and input state.

**Message list** — `ScrollArea` filling available height.

Each turn renders:
- **User message block** — right-aligned bubble (`bg-primary/15 text-primary-foreground` rounded), attribution line below: shadcn `Avatar` (17px) + name + `·` + time (`text-muted-foreground`).
- **Tool call separator** — centered `Separator` with a `Badge variant="outline"` showing "N tool calls · Xs ›". Clicking expands the tool call log (out of scope for v0).
- **AI message block** — left-aligned bubble (`bg-card border` rounded), attribution line: AI `Avatar` + "Claude · {model}" + time.

**Input area** — bottom of main panel, separated by `border-t border-border`.

Uses Vercel AI SDK `ChatInput` (wraps `Textarea`):
- Auto-grows up to ~120px.
- Footer row with:
  - Model selector pill — shadcn `Button variant="outline" size="sm"`.
  - Token usage pill — shadcn `Badge variant="outline"` with a `Progress` bar (3px).
  - Max mode toggle — shadcn `Button variant="outline" size="sm"`.
  - Send button — shadcn `Button size="icon"` with `bg-primary`.

---

## Routing

```
/                         → redirect to first project
/projects                 → project list (future)
/projects/:projectId      → AppShell with sidebar for that project
  /chats/:chatId          → project-level chat view
  /repos/:repoId/workspaces/:workspaceId/:step   → workspace chat view
```

TanStack Router file-based routes under `web/src/routes/`.

---

## Component File Structure

```
web/src/
  components/
    layout/
      AppShell.tsx          # two-panel shell + resize logic
      Sidebar.tsx           # sidebar scroll area + row composition
      SidebarHeader.tsx     # breadcrumb + avatar
      SidebarRows.tsx       # ChatRow, RepoRow, WorkspaceRow, NewRow
    chat/
      ChatView.tsx          # message list + input
      MessageBubble.tsx     # user and AI bubble variants
      ChatInput.tsx         # input area with model/token/send
      ToolCallSeparator.tsx # centered separator between turns
    ui/                     # shadcn/ui generated components (do not edit)
  domain/
    health.ts               # existing
  lib/
    transport.ts            # existing
    useEvents.ts            # existing
    query.ts                # existing
  routes/
    __root.tsx
    index.tsx
```

---

## Installation Steps

1. Install shadcn/ui canary with Tailwind v4 support:
   ```bash
   cd web && npx shadcn@canary init
   ```
   Choose: dark theme, CSS variables, `src/` path alias.

2. Add required shadcn components:
   ```bash
   npx shadcn@canary add avatar badge button progress scroll-area separator tabs textarea
   ```

3. Install Vercel AI SDK:
   ```bash
   npm install ai @ai-sdk/react
   ```

4. Wire `useChat` to the existing `/api/v0/` transport (stream endpoint TBD in backend spec).

---

## Constraints

- No hardcoded colors — only semantic Tailwind tokens.
- No custom CSS classes beyond what shadcn/ui generates.
- shadcn `ui/` components are never modified directly.
- All sidebar widths, row heights, and spacing use Tailwind scale values (`h-9`, `px-2`, `my-0.5`, `rounded-lg`), not arbitrary values.
