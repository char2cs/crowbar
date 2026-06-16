# OOBE Empty States Design

**Date:** 2026-06-16  
**Status:** Approved  
**Scope:** Two empty-state screens that form the out-of-box experience for new Crowbar users.

---

## Overview

A new Crowbar user hits two distinct empty states in sequence:

1. **No project** — the app opens with no project folder configured. The sidebar is absent.
2. **Project added, no repository** — a project folder exists, the sidebar appears, but no git repo has been added yet.

Both screens replace their current placeholder text blobs with the `@coss/empty` component pattern (already at `components/ui/empty.tsx`) using the `icon` variant — the app icon in a rounded card with two ghost cards rotated behind it.

---

## Screen 1 — No Project (OOBE Entry)

### Layout

The `ProjectListPage` currently renders a `Projects` header and an `+ Import project` button even when there are no projects. When `projects.length === 0`, both should be hidden — the entire view should be the empty state centered in a full-bleed canvas with no chrome above it.

The window has no sidebar (that's already the case today). The empty state fills the full content area.

### Component structure

```tsx
<Empty>
  <EmptyMedia variant="icon">
    <img src={appIconUrl} width={40} height={40} style={{ borderRadius: 10 }} />
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

Opens a small popover or tooltip (not a modal) that explains in 2–3 lines:

> A project is a local folder — the home for your repositories and cross-repo AI agents. Think of it like a workspace root: add repos inside it, then open them as workspaces to review code, run terminals, and chat with agents that understand your whole architecture.

No separate page or route needed.

### App icon source

Use the Tauri app icon already bundled at `desktop/src-tauri/icons/128x128.png`. For the web layer, expose it as a static asset (copy to `web/public/` or reference via the Tauri asset protocol) so it can be used as an `<img>` src.

---

## Screen 2 — Project Added, No Repository

### When it appears

After the user adds a project, the sidebar becomes visible. The main content area shows this state when no workspace (repository) has been added to the project yet.

### Layout

Standard IDE shell with sidebar — identical to the normal workspace view, but the right-hand content panel renders the empty state instead of a workspace.

### Component structure

```tsx
<Empty>
  <EmptyMedia variant="icon">
    {/* git branch icon, lucide GitBranch or similar */}
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

The sidebar shows the project name and a `+ Add repository` item under the Workspaces section (dashed border, muted style) that triggers the same action as the button in the main area.

---

## What doesn't change

- The `ImportProjectModal` (triggered by "Choose folder") is unchanged.
- The add-repository flow is unchanged.
- Routing and state logic are unchanged — these are purely presentational replacements.
- The `ProjectListPage` header (`Projects` + `+ Import project` button) is only hidden in the zero-projects case; it reappears normally once projects exist.

---

## Files to touch

| File | Change |
|------|--------|
| `web/src/components/projects/project-list-page.tsx` | Replace inline empty state with `<Empty>` pattern; hide header when `projects.length === 0` |
| `web/src/components/layout/ide-shell.tsx` | Replace "Select project" / no-workspace placeholder with Screen 2 `<Empty>` pattern |
| `web/public/` (or equivalent) | Add `icon.png` (128×128 app icon) as a static asset |
