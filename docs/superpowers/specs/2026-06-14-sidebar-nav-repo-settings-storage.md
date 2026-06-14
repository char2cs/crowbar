# Sidebar Navigation + Repo Settings + Storage Unification — Design Spec

## Overview

One PR that delivers three tightly coupled changes:

1. **Backend storage unification** — all Crowbar state centralised under `~/.crowbar/`; worktrees move from a sibling `.crowbar-worktrees/` pattern to `~/.crowbar/projects/<host>/<owner>/<repo>/workspaces/<ws-id>/`; repo icons saved to the same per-repo directory.
2. **Sidebar push navigation** — a globally accessible `useSidebarNavStore` that lets any component push an arbitrary screen into the sidebar with an iOS-style push/pop transition.
3. **Repo settings inline panel** — the existing `<Sheet>` overlay is replaced by a nav-stack screen pushed into the sidebar; adds an icon picker (upload / emoji / GitHub avatar) on top of the existing branch-import UI.

---

## 1. Backend — Storage Layout

### New directory structure

```
~/.crowbar/
├── state/                                      ← unchanged
│   ├── store/crowbar.db
│   └── events/*.db
├── projects/
│   └── <host>/<owner>/<repo>/                  ← e.g. github.com/acme/quiver.desktop
│       ├── icon.<ext>                          ← uploaded repo image (png/jpg/webp)
│       └── workspaces/
│           └── <workspace-id>/                 ← git worktree
└── crowbar.sock                                ← unchanged
```

The per-repo path is derived from the repository's remote URL, making it stable regardless of where the repo is cloned on disk. The `<host>/<owner>/<repo>` segments are the URL hostname + path components with any trailing `.git` stripped.

### worktreepath package

`api/internal/app/usecases/internal/worktreepath/worktreepath.go`

**Current signature:**
```go
func For(repoPath string, branch string) string
```

**New signature:**
```go
func For(repoURL string, workspaceID string) string
```

Path formula:
```
<crowbarHome>/projects/<host>/<owner>/<repo>/workspaces/<workspace-id>
```

Where `crowbarHome` is `~/.crowbar` (resolved via `os.UserHomeDir()`).

The workspace ID is already a stable UUID assigned at creation time — no hashing needed.

### CreateWorkspace usecase

Currently calls `worktreepath.For(repo.LocalPath, input.Branch)`. Change to `worktreepath.For(repo.RemoteURL, workspace.ID)`. The repo's `RemoteURL` is already stored in the DB.

### Icon endpoints

**`PUT /v0/repos/:id/icon`**
- Accepts `multipart/form-data` with field `icon` (image file, max 5 MB)
- Accepted MIME types: `image/png`, `image/jpeg`, `image/webp`
- Saves to `~/.crowbar/projects/<host>/<owner>/<repo>/icon.<ext>`
- Replaces any existing icon file for that repo
- Updates `repositories.avatar_url` to the local file path
- Returns `{"avatarUrl": "/v0/repos/:id/icon"}`

**`GET /v0/repos/:id/icon`**
- Already exists; ensure it resolves local file paths (not just remote URLs)
- Serve with appropriate `Content-Type` based on file extension
- Return 404 if no icon file exists

**`DELETE /v0/repos/:id/icon`** (reset to default)
- Deletes the icon file
- Clears `avatar_url` in DB
- Returns 204

**`PUT /v0/repos/:id/icon/emoji`**
- Body: `{"emoji": "🦊"}` (single emoji character, validated server-side)
- Stores the emoji string with a `emoji:` prefix in `avatar_url`: e.g. `emoji:🦊`
- This avoids ambiguity with HTTP URLs and local paths

### avatar_url interpretation (frontend + backend)

The `avatarUrl` field returned by `/v0/repos` and `/v0/workspaces` follows one of three formats:

| Format | Example | Frontend rendering |
|---|---|---|
| HTTPS URL | `https://avatars.githubusercontent.com/u/…` | `<img src={url}>` |
| API path | `/v0/repos/:id/icon` | `<img src={apiFetch(path)}>` (proxied via Tauri) |
| Emoji string | `emoji:🦊` | Strip prefix, render as `<span>{emoji}</span>` |
| Empty / absent | `""` | Fallback to generated initials + colour |

The backend sets `avatar_url` to the appropriate format on each write endpoint; the frontend reads it as-is and branches on the format.

### Existing worktrees

Dev-only machines: clear `.crowbar-worktrees/` directories and reimport workspaces. No migration code needed (pre-production, no users).

---

## 2. Frontend — Sidebar Navigation System

### useSidebarNavStore

New store at `web/src/features/sidebar/stores/sidebar-nav.ts`.

```ts
interface SidebarScreen {
  id: string          // unique key, e.g. 'repo-settings:abc123'
  title: string       // shown next to the back button
  component: ReactNode
}

interface SidebarNavStore {
  stack: SidebarScreen[]
  push: (screen: SidebarScreen) => void
  pop: () => void
  reset: () => void
}
```

Rules:
- `push` appends to the stack; if a screen with the same `id` is already on the stack, it is a no-op (prevents duplicates from double-clicks).
- `pop` removes the last item; no-op if stack is empty.
- `reset` clears the stack entirely (used on workspace navigation).

### NavStack / NavScreen components

New file: `web/src/features/sidebar/components/nav-stack.tsx`

`NavStack` clips overflow (`overflow: hidden`, `position: relative`) and renders its children as absolutely-positioned `NavScreen` layers.

`NavScreen` applies CSS transform classes based on its position in the stack:
- Top of stack (active): `translateX(0)`
- Any screen below top: `translateX(-25%)` + `opacity: 0.4`
- Transition: `transform 280ms cubic-bezier(0.4, 0, 0.2, 1)` + `opacity 200ms ease`

The back button is rendered by `NavStack` in a header bar when `stack.length > 0`, showing `stack[stack.length - 1].title`. Clicking back calls `useSidebarNavStore.getState().pop()`.

### Sidebar integration

`web/src/components/layout/sidebar.tsx` (or wherever `WorkspaceTree` is mounted) wraps content in `NavStack`:

```tsx
function SidebarContent() {
  const stack = useSidebarNavStore(s => s.stack)
  return (
    <NavStack>
      <WorkspaceTree />
      {stack.map(screen => (
        <NavScreen key={screen.id}>{screen.component}</NavScreen>
      ))}
    </NavStack>
  )
}
```

The `WorkspaceTree` is never unmounted — it stays as the persistent root layer.

---

## 3. Frontend — RepoSettingsPanel (inline)

### What changes

- Remove `<Sheet>`, `<SheetPopup>`, `<SheetHeader>`, `<SheetTitle>` wrappers.
- Remove `open` / `onOpenChange` props.
- The component becomes a plain scrollable div that fills whatever container it's placed in.
- `workspace-tree.tsx` no longer mounts `<RepoSettingsPanel>` at the bottom. Instead the gear click calls:

```ts
useSidebarNavStore.getState().push({
  id: `repo-settings:${repo.id}`,
  title: repo.name,
  component: <RepoSettingsPanel repoId={repo.id} repoName={repo.name} />,
})
```

### Icon section (new, added above the branch import section)

Three actions rendered as a compact picker row:

**Upload image**
- Hidden `<input type="file" accept="image/png,image/jpeg,image/webp" />`
- On change: `PUT /v0/repos/:id/icon` with multipart body
- On success: invalidate/refetch `useWorkspaceListStore` so the sidebar avatar updates immediately

**Pick emoji**
- Single-character text input
- On submit (enter or blur with valid emoji): `PUT /v0/repos/:id/icon/emoji`
- Validated client-side: input must be exactly one Unicode emoji codepoint

**Use GitHub avatar**
- Button that calls `PUT /v0/repos/:id/icon/github`
- Backend: looks up the repo's `RemoteURL`, calls the GitHub API (`GET /repos/<owner>/<repo>`) to get `avatar_url`, stores it in `repositories.avatar_url` (remote HTTPS URL, no local file saved)
- On success: refetch workspace list

**Reset to default**
- Small "×" or "Reset" link visible only when a custom icon is set
- Calls `DELETE /v0/repos/:id/icon`

### Unchanged

- Branch filter input
- Protected / other branch lists
- Per-branch checkboxes
- Import button + `handleImport` logic (try/finally, fetch after)
- Danger Zone (remove repo)

---

## Testing

### Backend

- `TestWorktreePath_NewFormula` — unit test for the new `worktreepath.For(repoURL, wsID)` formula
- `TestRegression_WorkspaceCreatesUnderCrowbarHome` — integration test asserting worktree dir is under `~/.crowbar/projects/`
- `TestRegression_IconUploadSavesToDisk` — integration test: upload PNG → file exists at expected path, `avatar_url` updated in DB
- `TestRegression_EmojiIconStored` — emoji string stored in `avatar_url`, no file written

### Frontend

- `useSidebarNavStore` — push/pop/reset unit tests, duplicate-id no-op
- `NavStack` — renders correct transform classes for 0, 1, 2 screens on stack
- `RepoSettingsPanel` — renders without Sheet; icon section present; upload triggers PUT

---

## Out of scope

- Multi-project support — path is `~/.crowbar/projects/<host>/<owner>/<repo>/` with no project UUID level for now; UUID nesting deferred until multi-project is designed
- GitLab / self-hosted remote URL parsing (GitHub format only for the path derivation)
- Nested nav (more than one level of push beyond the repo settings screen)
