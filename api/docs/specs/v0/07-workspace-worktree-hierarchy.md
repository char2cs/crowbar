# Crowbar Backend — Workspace & Worktree Hierarchy

> **Status:** Approved
> **Date:** 2026-06-03
> **Depends on:** `00-architecture-and-domain.md`, `04-git-subsystem.md`
> **Scope:** The workspace = worktree + branch model, the locked/unlocked
> distinction, local child→parent merge, and child re-parenting. This is one of
> Crowbar's signature features. Covers UX spec §2, §22 (worktrees), §24
> (conflicts during merge/re-parent).

---

## 1. Workspace = Git Worktree + Branch

Every workspace is a **`git worktree`** (its own directory on disk) checked out
to a **branch**. The worktree directory is the repo path that the file tree, git
status, terminals, and editors all operate against.

```
Repository (one clone on disk)
  └─ Workspace = git worktree @ <path>, on branch <branch>
       └─ child Workspace = git worktree @ <path2>, on branch <branch2>
            └─ ... (unbounded depth)
```

Creating a child workspace:

```
git worktree add <childPath> -b <childBranch> <parentBranch>
```

The child branches off the **parent's current tip**. That tip's SHA is recorded
as the child's **`forkPointSha`** (`00` §5.3) at creation time — it is the
authoritative fork point for re-parenting (§4), never recomputed via `merge-base`.
Deleting a workspace runs `git worktree remove <path>` (and deletes the branch if
Crowbar-managed). The `parentId` field on the Workspace aggregate tracks the tree.

> **Org/Project context.** Above `Repository` in the sidebar sits the **Project**
> (`00` §5.1) — the "Org name" node and org-switcher. It is just a top-level
> grouping of repos; there is no separate Org entity.

---

## 2. Two Classes of Workspace

|  | **Protected** (locked) | **Crowbar-managed** (unlocked) |
|--|------------------------|--------------------------------|
| Origin | Maps to a provider protected branch (`main`, `develop`) | Created locally in Crowbar |
| `locked` | `true` by default | `false` |
| Receives **local** merges? | **No** | **Yes** |
| Deletable? | No | Yes |
| Allowed operations | **Chat only** | Full (edit, commit, merge children, re-parent) |
| Integration path | **Platform PR** (provider — see `08`) | **Local merge** from children |

`locked` is set at creation by consulting the Git Provider engine
(`08-git-provider-engine.md`): if the branch is provider-protected (or matches
the fallback config list when no provider access), the workspace is locked.
Locked is a flag, not a status (`00` §6.1).

A locked workspace permits **only chat creation** — no file writes, no commits,
no receiving merges. It exists so the user can reason about / discuss a protected
branch without being able to mutate it outside the provider's PR process.

---

## 3. Two Distinct "Merges"

The single most important distinction in this subsystem: there are **two
completely different operations** the UI both calls "merge."

### 3.1 Local merge (child → unlocked parent)

Pure local git, **Crowbar-managed, invisible to the provider**. The platform
never knows the child existed.

**Preconditions:**
- Parent is **unlocked**.
- Child has **no unresolved conflicts** with the parent. If merging would
  conflict, the child must resolve them **first** (conflict UI, §24 /
  `04-git-subsystem.md` §6) before the merge is allowed.

**Operation** (run in the parent's worktree), per the merge-strategy selector:

| Strategy | Command |
|----------|---------|
| merge  | `git merge <childBranch>` |
| squash | `git merge --squash <childBranch>` && `git commit` |
| rebase | `git rebase <childBranch>` (replays onto parent) |

Never pushed unless the user later pushes the parent. After a successful local
merge the child may be kept or deleted (user choice).

```
POST /v0/workspaces/:childId/merge-into-parent { strategy }
```

Guarded: returns an error if the parent is locked or the child has unresolved
conflicts.

### 3.2 Platform PR (→ protected parent)

A locked branch **cannot** receive a local merge. The only way changes reach a
protected branch is a **real pull request on the provider** — and Crowbar does
**not** create it (the user or an agent does, via `gh`/`glab`/web). Crowbar only
**reads** that PR's state. See `08-git-provider-engine.md`.

---

## 4. Re-parenting (child migrates to a new parent)

A **leaf** child workspace can migrate to a different parent. The git operation:

```
git rebase --onto <newParentTip> <forkPointSha> <childBranch>
```

`forkPointSha` is the commit the child branch was **created from**, recorded on
the Workspace aggregate at `git worktree add` time (`00` §5.3). We use the
**recorded SHA**, not `git merge-base` — because once a local merge (§3.1) has
happened between the child and its parent, `merge-base` can return a commit that
includes parent history, and `rebase --onto` would then drop legitimate child
commits or replay parent commits. The recorded fork point is robust to prior
merges.

This replays **only the child's own commits** onto the new parent's tip,
dropping the old parent's history from underneath.

- **Conflicts** during the rebase surface in the standard conflict UI (§24); the
  child must resolve them before migration completes.
- It **rewrites the child's commit SHAs** — safe, because the child is local and
  Crowbar-managed (never been on the provider).
- On success, set `parentId = newParentId`, **update `forkPointSha` to the new
  parent's tip** (the new branch-creation point), and re-broadcast.
- **Guard — new parent unlocked:** re-parenting onto a protected branch is invalid
  (that path is a PR, not a local rebase).
- **Guard — child must be a leaf:** re-parenting a workspace that **has its own
  children is forbidden** in v0. The rebase rewrites the child's SHAs, which would
  orphan every descendant's `forkPointSha` (they branched off commits that no
  longer exist) and corrupt their merge bases. The user must re-parent or detach
  descendants first. (Cascade-rebasing a whole subtree may be added later.)

```
POST /v0/workspaces/:childId/reparent { newParentId }   // 409 if child has children
```

### 4.1 The fork-point invariant (applies beyond re-parent)

The rule re-parent enforces is one instance of a general invariant:

> A workspace's `forkPointSha` must remain a **reachable ancestor of its own
> branch**. Any operation that **rewrites a node's own committed history below a
> descendant's fork point** orphans that descendant.

Classifying the mutations:

- **Append-only — always safe** (never rewrite existing history a descendant
  forked from): local `merge` and `squash` merges *received by* a node (§3.1 —
  they add commits on top), ordinary commits, and `git reset` (moves a ref but the
  forked commit objects remain reachable from the descendant).
- **History-rewriting — restricted to leaf nodes** (same guard as re-parent): the
  `rebase` merge strategy (§3.1) and re-parent (§4), both of which rewrite the
  node's own commit SHAs. If the node **has children**, these are **forbidden** in
  v0 (the operation returns 409); the user detaches/re-parents descendants first.

This keeps `forkPointSha` valid for every node without a cascade-repair pass in
v0. (Subtree cascade-rebase may relax this later.)

---

## 5. Deletion & Cascade — UX §2, §15

Dragging a workspace to "Drop to delete" cascades to children, **skipping
locked** ones (a locked child blocks its own deletion and is left in place;
unlocked descendants are removed). Each removal:

```
git worktree remove <path>      # and delete the branch if Crowbar-managed
```

Cascade is computed over the `parentId` tree. The Workspace aggregate's delete
command enforces the locked-skip rule.

---

## 6. State & Real-time

All hierarchy mutations (create child, local merge, re-parent, delete) go
through the **Workspace Asynx aggregate** and broadcast on the global Workspaces
topic (`03-realtime-websockets.md`), so the sidebar tree stays live. The
`+N/-N`, `hasConflicts`, and `agent-running` overlays are driven as described in
`00` §6.1 and the git/fs specs.

---

## 7. REST Surface (hierarchy-specific)

```
POST   /v0/workspaces                       create { repoId, branch, parentId? }
                                            (locked resolved via provider engine)
DELETE /v0/workspaces/:id                   delete (cascade, skip locked)
POST   /v0/workspaces/:childId/merge-into-parent { strategy }   local merge
POST   /v0/workspaces/:childId/reparent          { newParentId }  rebase --onto
```

(Generic workspace read routes are in `02-api-surface.md`.)

---

## 8. Out of Scope

- Creating PRs on the provider (never ours — `08`).
- Pushing local merges to a remote (only happens if the user pushes the parent
  explicitly via the git subsystem).
