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
- **Git** — the branch's commit diff (the same `MultiFileDiff` viewer as §10 /
  `04-git-subsystem.md`).
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

BranchChat { id, title, age, isActive }   // lightweight; lifecycle = chat spec
```

- `diff` comes from the git engine: `git diff <base>...<branch>`.
- `threads` come from the ReviewThread Asynx repo (§3).
- `mergeStrategy` is stored on the workspace (set via `PATCH .../review`).
- `description` — see §5.

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
(`11-agentic-bridge-spike.md`) and is therefore **deferred**.

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

ReviewThread changes flow through the standard Asynx → hub path. Thread
updates can ride the Workspaces topic (the review panel is workspace-scoped) or a
dedicated review broadcast if needed; the default is to refresh the review read
model on thread mutation and let the panel re-fetch, since review editing is
low-frequency and not latency-critical (unlike git status or terminal output).

---

## 8. Out of Scope

- PR creation/merge on the provider (`08`).
- AI description generation (bridge spike).
- BranchChat conversation content (chat spec + bridge spike).
