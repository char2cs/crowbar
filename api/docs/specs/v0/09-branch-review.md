# Crowbar Backend — Branch Review

> **Status:** Approved
> **Date:** 2026-06-03
> **Depends on:** `00-architecture-and-domain.md`, `04-git-subsystem.md`,
> `07-workspace-worktree-hierarchy.md`, `08-git-provider-engine.md`
> **Scope:** The branch review surface — About / Git / Discussion tabs and inline
> review threads. Covers UX spec §11. PR *creation* is never ours (`08`).

---

## 1. Overview

The branch review panel is the "review before merge" surface for a workspace's
branch. It has three subtabs:

- **About** — an AI-generated markdown description of what the branch does.
- **Git** — the branch's commit diff (the same `MultiFileDiff` viewer as UX §10;
  shape defined in `04-git-subsystem.md` §3).
- **Discussion** — inline comment threads anchored to specific diff lines.

A merge-strategy selector (merge / squash / rebase) sits in the header; it feeds
the **local merge** operation (`07-workspace-worktree-hierarchy.md` §3.1).

---

## 2. The BranchReview Read Model

`GET /v0/workspaces/:wsId/review` returns a **composite projection**, not a
stored entity. The usecase assembles it from several sources:

```
BranchReview {
  description    string                 // AI-generated (see §5 — bridge-deferred)
  mergeStrategy  merge | squash | rebase
  diff           MultiFileDiff          // git engine: branch vs base
  threads        ReviewThread[]         // ReviewThread aggregate
  conversations  BranchChat[]           // branch-scoped discussions
}

BranchChat { id, title, age, isActive }   // a Chat (01), surfaced read-only here
```

- `diff` comes from the git engine: `git diff <base>...<branch>` (three-dot, from
  the merge-base), where **`<base>` is the parent workspace's branch** (the
  `parentId` branch; for a root workspace with no `parentId`, the repo's
  `defaultBranch`, `00` §5.2). This three-dot review diff and the sidebar's
  `+N/-N` (`git diff --numstat <forkPointSha>`, fork point → working tree, `00`
  §5.3) can diverge once the parent advances past the fork point — intentional:
  the sidebar shows "lines this branch added since it forked," while the review
  shows "the change set vs the current base."
- `threads` come from the ReviewThread Asynx repo (§3).
- `mergeStrategy` is stored on the workspace (set via `PATCH .../review`).
- `description` — see §5.
- **`conversations` are just `Chat`s** (`01-chat-lifecycle.md`) belonging to the
  workspace, projected into the panel as `BranchChat` (a lightweight view — `id`,
  `title`, `age`, `isActive`). There is **no separate branch-chat entity**.
  "Start a new branch-scoped discussion" (UX §11) reuses the existing
  `POST /v0/workspaces/:wsId/chats` route — no new endpoint. (A future `type`
  marker could distinguish review discussions from main chats; not needed for v0.)

---

## 3. ReviewThread Aggregate (Asynx)

Inline comment threads anchored to diff lines. State machine: `open ↔ resolved`
(`00-architecture-and-domain.md` §6.3).

```
ReviewThread {
  id         uuid
  wsId       uuid
  filePath   string
  lineNumber int
  side       left | right        // which side of the diff
  isResolved bool
  messages   ReviewMessage[]      // append-only within the aggregate
}

ReviewMessage {
  id        uuid
  author    string?               // null = current user
  isAgent   bool
  body      string                // markdown
  createdAt time                  // ISO 8601
}
```

Unlike chat turns, a thread is **bounded** (a handful of messages), so keeping
messages **inside the aggregate** is appropriate — no separate content table.

### Commands

| Op | Endpoint | Command | Transition |
|----|----------|---------|------------|
| Post comment (opens thread) | `POST /v0/workspaces/:wsId/review/threads` | `OpenThread` | → open |
| Reply | `POST /v0/workspaces/:wsId/review/threads/:id/reply` | `ReplyThread` | — |
| Resolve | `PATCH /v0/workspaces/:wsId/review/threads/:id { isResolved: true }` | `ResolveThread` | → resolved |
| Reopen | `PATCH /v0/workspaces/:wsId/review/threads/:id { isResolved: false }` | `ReopenThread` | → open |

`OpenThread` payload: `{ filePath, lineNumber, side, body }`. The first message is
created with the thread.

---

## 4. Merge Strategy

```
PATCH /v0/workspaces/:wsId/review { mergeStrategy }
```

Stored on the Workspace aggregate. It is the strategy used by the **local merge**
of this branch into its parent (`07` §3.1) — `merge` → `git merge`, `squash` →
`git merge --squash`, `rebase` → `git rebase`.

For a branch targeting a **protected** parent, the merge happens on the provider
via a PR the user/agent opens — Crowbar only reads that PR's state (`08`).

---

## 5. AI-Generated Description — Bridge-Deferred

The **About** tab description (and the PR-title/body pre-fill the frontend does
in UX §23) is "AI-generated" — it depends on an agent summarizing the branch
diff. That generation depends on the **Agentic Bridge**
(`12-agentic-bridge-spike.md`) and is therefore **deferred**.

For this spec:
- The `description` field **exists** in the read model.
- **How** it is produced is post-spike. Until then it may be empty, a manually
  set value, or a placeholder. No backend generation is specified here.

---

## 6. Pull Requests — Not Ours

Per `08-git-provider-engine.md`: Crowbar does **not** create or merge PRs on the
provider. The review panel's "Open Pull Request" action (UX §23) is a
**frontend → user/agent** flow (e.g. invoking `gh pr create`); the backend's only
role is to **read** the resulting PR state via the provider engine and reflect it
on the workspace badge.

There is no `POST /v0/.../pr` create endpoint. The workspace's `prUrl` / `status`
fields (`08` §5) are the PR surface the review panel renders.

---

## 7. Real-time

ReviewThread changes flow through the standard Asynx → hub path. The decision:
**re-fetch on mutation, no dedicated broadcaster.** On a thread mutation the
backend refreshes the review read model and the panel re-fetches
`GET /v0/workspaces/:wsId/review`. Review editing is low-frequency and not
latency-critical (unlike git status or terminal output), so it does **not**
warrant its own WS topic — this keeps the broadcaster set at the seven listed in
`03-realtime-websockets.md` §3 with no review-specific channel.

---

## 8. Out of Scope

- PR creation/merge on the provider (`08`).
- AI description generation (bridge spike).
- BranchChat conversation content (chat spec + bridge spike).
