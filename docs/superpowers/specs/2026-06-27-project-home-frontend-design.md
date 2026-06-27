# Project Home — Frontend Design Spec

**Date:** 2026-06-27
**Status:** Approved

## Goal

Surface the Project Home workspace in the UI: a sidebar row above the project indicator, a dedicated route, and a full workspace view with the Git tab suppressed.

---

## Context

The backend (PR on `hardening/production-readiness`) auto-provisions one home workspace per project at `GET /v0/projects/:projectId/home`. It returns a `WorkspaceDTO` with `kind: "home"`. The frontend currently has no awareness of this endpoint or workspace kind.

---

## Design

### 1. `WorkspaceDTO` — add `kind`

**File:** `web/src/lib/types.ts`

Add to `WorkspaceDTO`:
```ts
kind?: 'git' | 'home'
```

Absent field (legacy records) is treated as `'git'`.

---

### 2. Project Home sidebar row

**File (new):** `web/src/components/layout/project-home-row.tsx`

A single clickable row rendered **above** `ProjectSwitcherRow` ("Rabbyte") inside the Workspaces tab of the sidebar.

- **Icon:** `House` from `@phosphor-icons/react`, `size={14}`, `weight={isActive ? 'fill' : 'regular'}`
- **Label:** `"Home"`
- **Active state:** uses `ROW_ACTIVE` from `workspace-row-base.ts` when the current route matches `/ide/$projectId/home`; uses `ROW_INACTIVE` otherwise
- **Click:** `navigate({ to: '/ide/$projectId/home', params: { projectId } })`
- **`projectId`:** read from `useProjectStore((s) => s.activeProjectId)`
- **Active detection:** `useMatch({ from: '/ide/$projectId/home', shouldThrow: false })`

**Placement in sidebar:** `workspace-tree.tsx` renders `ProjectSwitcherRow` at line 105 then the repo tree. `ProjectHomeRow` is inserted directly before `ProjectSwitcherRow` in that file.

---

### 3. Route — `/ide/$projectId/home`

**File (new):** `web/src/routes/_shell/ide/$projectId/home.tsx`

TanStack Router picks this up automatically as a sibling to the `$repoId` directory.

**Route component responsibilities:**
1. Read `projectId` from route params
2. Call `GET /v0/projects/:projectId/home` — returns `WorkspaceDTO`
3. On success: render `WorkspaceView` with `wsId` from the response
4. On loading: render existing skeleton/spinner pattern
5. On error (404 or network): render a simple "Project Home unavailable" fallback

**Data fetching:** Use the same `useQuery` pattern as other workspace data fetches in the codebase. Cache key: `['home-workspace', projectId]`.

**`WorkspaceView`** is reused as-is — it accepts a `wsId` and renders file explorer, editor, terminals, and tabs. The git tab suppression (item 4) handles the rest.

---

### 4. Git tab suppressed on home route

**File:** `web/src/components/layout/sidebar-tab-bar.tsx`

The static `TABS` array is filtered before rendering. When the current route matches `/ide/$projectId/home`, the `'git'` entry is excluded:

```ts
const isHomeRoute = useMatch({ from: '/ide/$projectId/home', shouldThrow: false })
const visibleTabs = isHomeRoute ? TABS.filter((t) => t.tab !== 'git') : TABS
```

`visibleTabs` replaces `TABS` in the `.map()` call. No other changes to the component.

**Tab switching guard:** if the active tab is `'git'` when navigating to the home route, reset it to `'workspaces'` — add a `useEffect` in `SidebarTabBar` that fires when `isHomeRoute` becomes truthy.

---

### 5. Default redirect updated

**File:** `web/src/routes/_shell/index.tsx`

Current logic redirects to the first editable workspace under the active project. New logic:

1. If active project exists → redirect to `/ide/$projectId/home` (unconditionally — the home workspace always exists for any valid project)
2. No active project but projects exist → pick first project, redirect to its home
3. No projects → redirect to `/oobe` (unchanged)

The existing fallback logic (walking repos for an editable workspace) is removed — Project Home is always the safe landing.

---

## Files Changed

| File | Action |
|------|--------|
| `web/src/lib/types.ts` | Modify — add `kind` to `WorkspaceDTO` |
| `web/src/components/layout/project-home-row.tsx` | Create |
| `web/src/components/layout/workspace-tree.tsx` | Modify — insert `ProjectHomeRow` above `ProjectSwitcherRow` (line 105) |
| `web/src/routes/_shell/ide/$projectId/home.tsx` | Create |
| `web/src/components/layout/sidebar-tab-bar.tsx` | Modify — filter git tab + reset effect |
| `web/src/routes/_shell/index.tsx` | Modify — prefer home route redirect |

---

## Out of Scope

- Chat creation from Project Home (future)
- Search within Project Home (future — backend `/home/search` exists, frontend wiring deferred)
- Project Home settings / rename (future)
- PTY WebSocket for terminals in Project Home (backend deferred with TODO; frontend terminal panel will show create button but WS connect will fail gracefully until wired)
