# Sidebar Project Switcher — Design

Date: 2026-06-15
Status: Approved (pending spec review)

## Problem

Switching the active project today happens through a `DropdownMenu` in the
sidebar header (`SidebarProjectHeader`, the `Rabbyte ⌄` control). We want a
more integrated entry point: a thin switcher row at the top of the Workspaces
tab that opens a dedicated sidebar sub-screen for switching between and adding
projects — consistent with how repo settings already open as a pushed screen.

## Goals

- Replace the header dropdown with a thin entry-point row at the top of the
  Workspaces tab.
- Clicking the row opens a pushed sidebar screen (same `NavStack` mechanism as
  `RepoSettingsPanel`) listing projects and offering an "Import project" action.
- Selecting a project switches the active project and returns to the Workspaces
  list.
- Reuse existing primitives — no new store fields, no backend changes.

## Non-goals

- No new project-creation flow. "Add a project" reuses the existing
  `ImportProjectModal` (import a local folder).
- No change to the full-page `/projects` route (`ProjectListPage`); it stays as
  is for now.
- No change to the Chats / Files / Git tabs.

## Existing building blocks (reused as-is)

- `useSidebarNavStore` (`features/layout/stores/sidebar-nav.ts`) — push/pop
  screen stack.
- `NavStack` (`components/layout/nav-stack.tsx`) — renders pushed screens with a
  back button + `screen.title` header. The new screen supplies only its body and
  a title via `push({ id, title, component })`.
- `useProjectStore` (`lib/store/projects.ts`) — `projects`, `activeProjectId`,
  `setActiveProject`, `addProject`.
- `ImportProjectModal` (`components/projects/import-project-modal.tsx`) — existing
  add-project flow (`addProject` + backend repo/workspace creation + sidebar
  merge), as used by `ProjectListPage`.

## Design

### 1. Entry-point row (top of Workspaces tab)

- Rendered inside `WorkspaceTree`, as the first element above the repo list.
- Only appears on the Workspaces tab (it lives in the workspace tree, so it is
  naturally absent from Chats/Files/Git).
- Visual style per reference (Image #21): full-width, thin, a subtle bottom
  hairline (`border-b border-border` or equivalent token), muted text. Active
  project name on the left; a muted trailing affordance (chevron / label) on the
  right. Uses CSS variable tokens and `@/components/ui/*` per project conventions.
- On click: `useSidebarNavStore.getState().push({ id: 'project-switcher', title:
  'Projects', component: <ProjectSwitcherPanel /> })`.

### 2. Header cleanup (`SidebarProjectHeader`)

- Remove the project `DropdownMenu` (trigger + items + "Manage projects…").
- Remove the now-unused `onProjectsClick` / `onProjectSelect` props and update
  the call site in `ide-shell.tsx`.
- Keep the gear (settings) button, the `data-tauri-drag-region`, and the macOS
  traffic-light spacer.

### 3. `ProjectSwitcherPanel` (new pushed screen)

File: `components/layout/project-switcher-panel.tsx` (kebab-case file,
PascalCase export, per conventions).

- Body only — `NavStack` provides the back button + "Projects" title.
- **Project list**: each project rendered as a workspace-tree-style row
  (Image #24) — name + active-state marker. Read from
  `useProjectStore((s) => s.projects)` with a narrow selector.
  - Click → `setActiveProject(id)` then `useSidebarNavStore.getState().pop()`.
  - Active project (`activeProjectId`) visually marked.
- **"Import project" row** at the bottom → opens `ImportProjectModal`. On
  successful import, the modal's existing handler does `addProject` + refetch +
  sidebar merge; the panel then pops (or stays — see open questions resolved
  below: it stays open so the user sees the new project appear, matching
  `ProjectListPage` behavior).

### 4. Data flow

- All project data from `useProjectStore` (narrow selectors).
- Switching: `setActiveProject(id)` → existing reactive wiring updates the
  workspace tree (repos are filtered by `projectId`).
- Adding: delegated entirely to `ImportProjectModal`.

## Store / convention compliance

- Narrow `useProjectStore` selectors in render; `getState()` only in handlers.
- New component file kebab-case; no store→component imports introduced.

## Testing

- Unit/interaction test for `ProjectSwitcherPanel`: renders project rows, marks
  active project, clicking a row calls `setActiveProject` + `pop`, "Import
  project" opens the modal. Test lives in
  `web/src/__tests__/components/layout/project-switcher-panel.test.tsx` using
  `@/` imports.
- Update `ide-shell.test.tsx` if the `SidebarProjectHeader` prop change affects
  it.

## Decisions (resolved)

- "Add a project" = existing `ImportProjectModal`. (Confirmed.)
- Selecting a project pops back to the Workspaces list. (Confirmed.)
- After a successful import the switcher screen stays open so the new project is
  visible in the list (mirrors `ProjectListPage`).
