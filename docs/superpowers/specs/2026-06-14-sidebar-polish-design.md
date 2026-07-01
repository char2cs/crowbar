# Sidebar Polish — Design Spec

**Date:** 2026-06-14  
**Branch:** enhancement/ide-final-polish  
**Status:** Approved

---

## Overview

Five improvements to the workspace tree sidebar. Mostly backend (Go) with minor frontend changes. Covers GitHub and GitLab as providers.

---

## 1. Repo Icon Resolution

**Goal:** Show a meaningful icon for each repo instead of a generated letter badge.

### Backend

- Add `AvatarURL string` to `RepoDTO` (`api/internal/api/v0/dto/repo.go`).
- In `avatar.go` (`api/internal/app/usecases/internal/avatar/`), add a `ResolveAvatarURL(repoPath, remoteURL string) string` function that:
  1. Scans the repo filesystem for icon files in priority order:
     `favicon.svg → favicon.ico → favicon.png → logo.svg → logo.png → public/logo.* → src/assets/logo.*`
  2. If a file is found, returns the absolute filesystem path. The backend exposes these via a new `GET /v0/repos/:repoId/avatar` endpoint that reads the file from disk and streams it with the correct `Content-Type`. The frontend uses this URL (e.g. `/v0/repos/abc/avatar`) in the `<img src>`.
  3. If nothing is found, calls the GitHub/GitLab owner avatar API (org or user who owns the repo) and returns the HTTPS URL. Cache this URL on the repo record to avoid repeated API calls.
- Call `ResolveAvatarURL` during `importOneRepo()` and whenever the repo record is refreshed.
- `AvatarLabel` and `AvatarColor` remain on `RepoDTO` as fallback fields.

### Frontend

- In the repo row component (`workspace-tree.tsx`), if `repo.avatarURL` is set, render a 20×20 `<img>` with `border-radius: 4px` and `object-fit: cover`.
- Otherwise fall back to the existing letter badge (no change to current rendering).

---

## 2 & 3. Protected Branch Auto-Import + Import Workspaces Panel

**Goal:** All GitHub/GitLab protected branches appear as locked workspaces automatically. Non-protected branches require explicit user action to import.

### Backend — Auto-import protected branches

- In `importOneRepo()` (`api/internal/app/usecases/project/project_import.go`), after `adoptWorktrees()`, call `provider.ProtectedBranches(repoPath)` and for each result, call `POST /v0/workspaces` logic (or the usecase directly) to create a workspace record if one doesn't already exist. These workspaces are created with `locked=true`.
- **No new git worktree is created on disk at this point.** The workspace record is persisted with `worktreePath = ""` (empty). The existing `CreateChild` usecase already handles the default-branch case by adopting the repo path — this extends that pattern: a workspace with an empty `worktreePath` gets its worktree created lazily the first time the user opens it (i.e., when `GET /v0/workspaces/:id` is called and the worktree doesn't exist yet).
- Add a periodic `SyncProtectedBranches` Asynx job that re-runs the same logic for existing repos to pick up newly protected branches.

### Backend — Branch list endpoint

- New endpoint: `GET /v0/repos/:repoId/branches`
- Returns all remote branches via `git branch -r --format=%(refname:short)`, annotated with:
  ```json
  [
    { "name": "main", "isProtected": true, "hasWorkspace": true },
    { "name": "feature/quiver-shell", "isProtected": false, "hasWorkspace": true },
    { "name": "feature/redesign", "isProtected": false, "hasWorkspace": false }
  ]
  ```
- `isProtected` is resolved via `provider.ProtectedBranches()` (same call used elsewhere; result can be cached per-request).

### Frontend — Import Workspaces panel

- The repo settings panel (opened via the gear icon, see Feature 5) includes an **Import Workspaces** section.
- UI: checkbox multi-select list with a filter input.
  - Protected branches: pre-checked, disabled (greyed), labelled "auto-imported".
  - Non-protected branches: opt-in checkboxes. Already-imported branches shown as "imported" (no checkbox).
  - "Import N branches" button at the bottom calls `POST /v0/workspaces` for each selected branch.
- Data: fetched from `GET /v0/repos/:repoId/branches` when the panel opens.

### Behavior change on project import

- `adoptWorktrees()` continues adopting existing local worktrees regardless of protection status (no regression).
- Protected branch auto-import runs **after** `adoptWorktrees()` and is additive — it only creates missing records.
- Non-protected branches without local worktrees are no longer auto-imported; they appear in the Import Workspaces panel instead.

---

## 4. PR-Based Parent-Child Hierarchy

**Goal:** If a branch has an open PR targeting branch B, its workspace automatically becomes a child of B's workspace.

### Backend

- In `sync_provider_state.go`, after setting `PRTargetBranch`:
  1. Look up all workspaces in the same repo.
  2. Find the workspace whose `branch == PRTargetBranch`.
  3. If found **and** the current workspace's `parentId` is empty, set `parentId` to that workspace's ID and persist the update.
- **Sync does not override manually set `parentId`.** If the user has already dragged the workspace to a different parent (or parentId is already populated), the sync skips the update.
- If the PR is closed or merged, `parentId` is left unchanged (no orphaning).
- Supports both GitHub PRs and GitLab MRs (both already populate `PRTargetBranch` via their respective provider implementations).

### Frontend

No changes required — the sidebar tree already renders parent-child relationships from `parentId`.

---

## 5. Repo Settings Gear (Option C — chevron swaps on hover)

**Goal:** Discoverable access to repo settings without cluttering the default sidebar state.

### Frontend

- On the repo row in `workspace-tree.tsx`, the expand/collapse chevron (`ChevronDown` / `ChevronRight`) swaps to a `Settings` (gear) icon when the row is hovered.
- Clicking the gear opens the repo settings panel (slide-in panel or modal — to be determined during implementation based on existing UI patterns).
- Clicking anywhere else on the row (icon, name area) toggles collapse as before.
- No always-visible gear — the default state remains clean.

---

## Data Flow Summary

```
Project import
  └─ importOneRepo()
       ├─ adoptWorktrees()           (existing local worktrees → workspaces)
       └─ autoImportProtectedBranches()
            └─ provider.ProtectedBranches() → create workspace records (locked=true, stub state)

Provider sync (periodic)
  └─ sync_provider_state.go
       ├─ update locked / status / PRUrl / PRTitle / PRTargetBranch
       └─ if PRTargetBranch set AND parentId empty → set parentId

Asynx job: SyncProtectedBranches
  └─ runs periodically per repo
       └─ same as autoImportProtectedBranches()

GET /v0/repos/:repoId/branches
  └─ git branch -r → annotate with isProtected + hasWorkspace

RepoDTO.AvatarURL
  └─ resolved at import/refresh via avatar.ResolveAvatarURL()
       ├─ filesystem scan (favicon → logo → public/ → src/assets/)
       └─ fallback: provider owner avatar API (cached on repo record)
```

---

## Out of Scope

- Removing protection from branches via Crowbar (read-only from provider).
- Manual override of PR-driven `parentId` via settings (drag-drop is sufficient).
- Serving local icon files through the backend (file:// path exposed to the Tauri webview is sufficient for desktop).
- GitLab avatar API differences (handled inside the existing provider abstraction).
