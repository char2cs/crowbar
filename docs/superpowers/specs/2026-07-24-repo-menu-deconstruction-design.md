# Repo Menu Deconstruction — Icon Popover + Import Modal + PR Auto-Parenting

**Date:** 2026-07-24
**Branch:** enhancement/restyling
**Status:** Design — awaiting review

## Problem

The repo row's gear button opens a single `RepoSettingsPanel` pushed onto the
sidebar nav-stack. That panel bundles two unrelated concerns into one full-screen
view: editing the repo **icon**, and **importing** remote branches as workspaces.

Two problems:

1. **UX** — the two concerns want different affordances. Icon editing belongs on
   the icon; import belongs on a dedicated action. A shared nav-stack panel makes
   both feel heavy and buried.
2. **Correctness (production bug)** — import never parents anything. `handleImport`
   fires `postWorkspace(branch)` with no `parentId`, and the create handler
   (`crud.go:135`) then forks every imported branch from the repo default branch.
   The PR data needed for parenting (`PRInfo.TargetBranch`) exists but is never
   consulted at import. Auto-parenting was simply never wired in.

## Goals

- Split the settings panel into **two focused interactions** without losing any
  functionality:
  - **Icon → CossUI Popover** anchored to the repo avatar, opened only by clicking
    the avatar, with a pencil hover affordance.
  - **Gear → Import icon-button → CossUI Dialog** holding the branch import UI.
- Make import **PR-aware**: an imported branch is parented under the workspace for
  its open PR's base branch; missing ancestors are created and the whole chain is
  parented up to a protected/default root.
- Both interactions must **perform really well**: lazy content, virtualized list,
  client-side hint math, no network on the render path.
- Surface a **small hint** of what an import will do
  ("Imports N branches · creates M parents").

## Non-Goals

- Repo **rename** stays exactly where it is (double-click the repo name). The icon
  popover edits *only* the icon.
- No change to merge/reparent/pull flows beyond what auto-parenting needs.
- No new npm dependency (`@tanstack/react-virtual` is already present).

---

## Part 1 — UI Deconstruction (web)

### 1.1 Icon Popover

**Trigger.** In `repo-section.tsx`, wrap the avatar block in a `PopoverTrigger`.
The avatar's click handler calls `stopPropagation` so clicking it opens the
popover instead of navigating to repo-home; the rest of the row still navigates.

**Hover affordance.** A small pencil badge is absolutely positioned over the icon
and revealed on hover via a nested `group/repo-icon` — so it appears **only** when
hovering the avatar, not anywhere on the row. It is suppressed while the agent
spinner is showing (`repo.defaultWorking`), since the avatar is then replaced by
the spinner and is not editable.

**Content.** New `web/src/components/layout/repo-icon-popover.tsx` holds the icon
controls extracted verbatim from `RepoSettingsPanel`: the preview avatar and the
Upload / Emoji / GitHub / Reset actions with their existing handlers
(`handleUpload`, `handleFileChange`, `handleEmojiSubmit`, `handleGithubAvatar`,
`handleResetIcon`, plus the `iconVersion` cache-buster and `isTauri` native-dialog
path). No behavior change; only the container moves from a nav-panel section to a
popover body.

### 1.2 Import Modal

**Trigger.** Replace the gear button at `repo-section.tsx:243` with an icon button
using `DownloadCloud` (lucide) + a tooltip "Import branches", keeping the existing
`ROW_SUB_ACTION` styling so the row's three trailing buttons stay visually uniform.

**Content.** New `web/src/components/layout/repo-import-dialog.tsx` (CossUI
`Dialog`) holds the branch search + list + Import button + hint, extracted from
`RepoSettingsPanel`'s branch-import section.

### 1.3 Removal

Delete `web/src/components/layout/repo-settings-panel.tsx` and its nav-stack push
in `repo-section.tsx` (`useSidebarNavStore.getState().push({ id: 'repo-settings:…' })`).
Both halves now live as overlay/popover. Search for any other reference to the
`repo-settings:` nav id and the `RepoSettingsPanel` import and remove them.

---

## Part 2 — Performance (web)

Performance is a first-class requirement for both interactions.

1. **Lazy content.** base-ui `Popover`/`Dialog` render their portalled body only
   while open. The branch/PR fetch fires on open (inside the mounted body), never
   on sidebar row mount — so the tree pays nothing for N repos' worth of branch
   data.
2. **Virtualized branch list.** Use `useVirtualizer` from `@tanstack/react-virtual`
   inside the modal's scroll container, mirroring
   `use-agent-chat-list-virtualizer.ts`. Fixed row height; only visible rows render.
   A repo with hundreds of branches costs a handful of DOM rows.
3. **Client-side filter over stable data.** Branches are fetched once per open.
   Typing in the search box filters the in-memory array and re-measures only
   visible rows; it never refetches.
4. **Don't block the list on the network.** `/branches` is a local `git branch -r`
   (fast) and renders the list immediately. The PR graph (`gh pr list`, network)
   loads in parallel and only enriches the hint — the list is fully usable before
   the graph arrives.
5. **Client-side hint math.** Once the PR graph is loaded, the parent-chain
   resolution and the hint recompute purely client-side on each selection toggle —
   zero per-toggle round-trips.

---

## Part 3 — PR-Based Auto-Parenting (api)

### 3.0 What already exists (do NOT rebuild)

A poll-driven auto-reparent already exists: `provider.maybeReparentFromPR`
(`app/usecases/provider/provider_sync.go`) + `workspace.SetParentFromPR`
(record-only parent update via the `SetParentFromPR` command). On each provider
poll (on-view + 5-min sweep), `SyncFromState` reparents a workspace under the
existing sibling matching its OPEN PR's target branch — but only when
`ParentID == ""` and only when that parent **already exists**. It does NOT
create a missing parent, and it is timing-dependent (fires on poll, not at
import). That is the production gap.

This work adds the **deterministic, import-time** half: create missing
ancestors and parent the whole chain at import. It reuses `CreateChild` (which
persists `ParentID`, worktree.go:161/230) and does **not** touch
`maybeReparentFromPR`. Because import sets `ParentID` explicitly, the poll's
`ParentID == ""` guard makes import-parented rows invisible to it — no conflict,
no double-parenting. The poll path remains as post-import maintenance.


### 3.1 Provider: list open PRs

Add to the `GitProvider` interface (`provider.go`) and both implementations:

```go
// OpenPullRequests returns the head→base graph of all open PRs for the repo,
// in a single provider call. Empty when the CLI is unavailable or none are open.
OpenPullRequests(ctx context.Context, repoPath string) ([]PRLink, error)
```

`PRLink = { Head, Base string; Number int; Status, URL, Title string }`.

- **GitHub** (`github.go`): one `gh pr list --state open --json
  number,state,url,title,headRefName,baseRefName`. Reuses the existing `prJSON`
  parser and `runGH`.
- **GitLab** (`gitlab.go`): the analogous `glab mr list` call; mirrors the existing
  single-branch MR lookup.

The `Engine` (`engine.go`) exposes `OpenPullRequests` passthrough.

### 3.2 PR graph endpoint (for the hint)

`GET /v0/projects/:projectId/repos/:repoId/pull-requests` →
`[{ head, base, number, status, url, title }]` for open PRs. Thin handler over
`Engine.OpenPullRequests`. Advisory data for the client hint only — never trusted
for correctness. Soft-fails to `[]` on CLI-absent/unauthenticated (same posture as
the icon/GitHub avatar path).

### 3.3 Batch import endpoint

`POST /v0/projects/:projectId/repos/:repoId/workspaces/import` with
`{ branches: string[] }`. Chosen over per-branch create because it dedups shared
parents and avoids create-races between siblings that need the same missing parent.

Resolution algorithm (server-side, authoritative):

1. Fetch the open-PR graph once (`Engine.OpenPullRequests`) → `base[head] = base`.
2. Load existing workspaces → `imported` set (branch → workspace id, non-default
   only, matching `hasWorkspace` semantics) and protected-branch set.
3. For each requested branch, walk the base chain
   `X → base[X] → base[base[X]] → …`, collecting ancestors, until a terminal:
   - an **existing workspace** (reuse its id as parent),
   - a **protected branch** (already a locked workspace — reuse),
   - the **default branch** (repo-home workspace — reuse), or
   - a base with **no PR** and no workspace → still created, parented under default,
     then terminate.
   Guard against cycles with a per-walk visited set; on a cycle, break and parent
   the current node under default.
4. Topologically order the union of {requested branches ∪ collected ancestors},
   parents before children, deduped across all requested branches.
5. Create each missing node via the existing `worktree.CreateChild` with the
   resolved `parentId`, in order. Nodes already imported (or created earlier in the
   same batch) are skipped. Reuses the existing `branchTaken` guard as the backstop.

The endpoint returns 202 and runs the resolution + creates in the background; each
created workspace arrives on the per-repo WS stream (same as today's single create).

### 3.4 Web wiring

- New API helpers: `getRepoPullRequests(projectId, repoId)` and
  `importBranches(projectId, repoId, branches[])`.
- `handleImport` in the modal calls `importBranches` once (replacing the
  `Promise.allSettled` loop of `postWorkspace`), then refreshes the branch list so
  freshly-imported branches flip to `hasWorkspace = true`.

### 3.5 The hint (client-side)

Given the fetched PR graph and the `imported` set, compute for the current
selection:

- **N** = number of selected branches.
- **M** = size of the union of missing ancestors across all selected branches,
  **excluding** already-imported branches, protected branches, the default branch,
  and the selected branches themselves.

Render a small muted line by the Import button:
`Imports N branch(es) · creates M parent branch(es)` (drop the second clause when
`M == 0`). This is advisory; the server re-resolves on import.

---

## Testing

- **Backend regression** (`api/tests`, integration tag, `TestRegression_*`): import
  a branch whose open PR targets a base branch that has **no** workspace yet →
  assert the base workspace is created, the imported branch's `parentId` points at
  it, and a multi-level chain (grandparent PR) is fully built and parented up to a
  protected/default root. A second test: importing a branch whose base is *already*
  a workspace reuses it (no duplicate, correct `parentId`).
- **Provider**: unit test `OpenPullRequests` parsing with a stubbed `gh` exec.
- **Web** (`web/src/__tests__/` mirror): the parent-chain/hint math (pure function)
  across the edge cases; the icon popover opens only on avatar click; the import
  modal renders a virtualized list and disables already-imported rows.
- **Live Tauri verification** (per project rule): open the popover from the avatar,
  confirm the pencil hover affordance, edit the icon; open the import modal, scroll
  a long branch list, toggle selections and watch the hint, run an import that
  creates a parent, and confirm the tree parents correctly in the sidebar.

## Files (anticipated)

**web**
- `components/layout/repo-section.tsx` — avatar popover trigger + pencil hover;
  gear → import icon-button + tooltip; remove nav-stack push.
- `components/layout/repo-icon-popover.tsx` — new; extracted icon controls.
- `components/layout/repo-import-dialog.tsx` — new; virtualized list + hint + import.
- `lib/import/parent-plan.ts` — new; pure parent-chain + hint computation (tested).
- `lib/api.ts` — `getRepoPullRequests`, `importBranches`.
- delete `components/layout/repo-settings-panel.tsx`.

**api**
- `internal/engine/provider/provider.go` + `types/types.go` — `OpenPullRequests`,
  `PRLink`.
- `internal/engine/provider/providers/github/github.go`,
  `.../gitlab/gitlab.go`, `.../engine.go` — implementations + passthrough.
- `internal/api/v0/endpoints/repos/handlers/repos.go` + `routes.go` —
  `GET …/pull-requests`.
- `internal/api/v0/endpoints/workspaces/handlers/` + `routes.go` —
  `POST …/workspaces/import` + resolver.

## Open Risks

- `gh pr list` latency on large repos — mitigated by fetching the graph off the
  list-render path and running import in the background (202).
- Provider CLI absent/unauthenticated → graph empty → hint shows 0 parents and
  import falls back to today's default-parent behavior. Acceptable degradation.
