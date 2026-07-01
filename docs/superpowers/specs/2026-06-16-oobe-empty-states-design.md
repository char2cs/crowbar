# OOBE Empty States Design

**Date:** 2026-06-16  
**Status:** Approved  
**Scope:** Two empty-state screens that form the out-of-box experience for new Crowbar users.

---

## Overview

A new Crowbar user hits two distinct empty states in sequence:

1. **No project** — the app opens with no project folder configured. Full-canvas, no sidebar, no shell chrome.
2. **Project added, no repository** — a project folder exists, the sidebar appears, but no git repo has been added yet.

Both screens use the `@coss/empty` component pattern (already at `components/ui/empty.tsx`) with the `icon` variant — icon in a rounded card with two ghost cards rotated behind it.

---

## Routing architecture

Currently `__root.tsx` unconditionally wraps every route in `IDEShell`, which always renders the right sidebar. Screen 1 requires a completely chrome-free full-canvas view, so the shell must not mount at all.

**The fix:** move `IDEShell` out of `__root.tsx` into a pathless layout route (`_shell.tsx`). The `/oobe` route becomes a sibling that bypasses the shell entirely.

### Route tree (after change)

```
__root.tsx          ← providers only (HydrationGate, ErrorBoundary, AppSyncProvider)
  _shell.tsx        ← pathless layout: mounts IDEShell, wraps all normal routes
    /               ← redirect logic (see below)
    /projects       ← ProjectListPage (Screen 2 empty state lives here)
    /workspaces/$wsId
    /workspaces/new
    /chat/$chatId
  /oobe             ← Screen 1, renders outside IDEShell entirely
```

### Index redirect logic

`routes/index.tsx` `beforeLoad`:
- No projects → redirect to `/oobe`
- Has projects + resolvable workspace → redirect to `/workspaces/$wsId`
- Has projects, no workspace → redirect to `/projects`

When the user adds their first project inside `/oobe`, navigate to `/projects`.

---

## Screen 1 — `/oobe` (No Project)

### Layout

Full-canvas, no sidebar, no titlebar chrome. The `Empty` component fills the entire window centered vertically and horizontally.

### Component structure

```tsx
<Empty>
  <EmptyMedia variant="icon">
    <img src="/icon.png" width={40} height={40} style={{ borderRadius: 10 }} />
  </EmptyMedia>
  <EmptyHeader>
    <EmptyTitle>Open a project folder</EmptyTitle>
    <EmptyDescription>Choose a local directory to get started.</EmptyDescription>
  </EmptyHeader>
  <EmptyContent>
    <Button className="w-full rounded-full" onClick={openImport}>Choose folder</Button>
    <button className="text-xs text-muted-foreground/50 underline underline-offset-2">
      What's a project?
    </button>
  </EmptyContent>
</Empty>
```

### "What's a project?" action

Opens a small popover (not a modal) explaining:

> A project is a local folder — the home for your repositories and cross-repo AI agents. Add repos inside it, then open them as workspaces to review code, run terminals, and chat with agents that understand your whole architecture.

### App icon

Copy `desktop/src-tauri/icons/128x128.png` to `web/public/icon.png` so it's available as a static asset.

### After import

On successful project import, navigate to `/projects`.

---

## Screen 2 — `/projects` empty state (Project Added, No Repository)

This renders inside the normal `IDEShell` with the sidebar visible.

### Component structure

```tsx
<Empty>
  <EmptyMedia variant="icon">
    <GitBranchIcon className="size-4.5 text-foreground" />
  </EmptyMedia>
  <EmptyHeader>
    <EmptyTitle>No repositories yet</EmptyTitle>
    <EmptyDescription>Add a git repository to open a workspace.</EmptyDescription>
  </EmptyHeader>
  <EmptyContent>
    <Button variant="outline" size="sm" className="rounded-full" onClick={openAddRepo}>
      Add repository
    </Button>
  </EmptyContent>
</Empty>
```

### Sidebar state

The sidebar shows the project name with a `+ Add repository` entry under Workspaces (dashed border, muted) that triggers the same action as the main CTA.

---

## What doesn't change

- `ImportProjectModal` and add-repository flow are unchanged.
- All existing workspace routes are unchanged — they move under `_shell.tsx` but their paths stay the same.

---

## Files to touch

| File | Change |
|------|--------|
| `web/src/routes/__root.tsx` | Remove `IDEShell`; keep only providers |
| `web/src/routes/_shell.tsx` | New pathless layout route — mounts `IDEShell` |
| `web/src/routes/oobe.tsx` | New route — Screen 1 full-canvas OOBE component |
| `web/src/routes/index.tsx` | Update redirect: no projects → `/oobe` |
| `web/src/components/projects/project-list-page.tsx` | Replace inline empty state with Screen 2 `<Empty>` pattern |
| `web/src/components/layout/ide-shell.tsx` | Replace "Select project" placeholder with Screen 2 `<Empty>` pattern |
| `web/public/icon.png` | Add 128×128 app icon as static asset |
