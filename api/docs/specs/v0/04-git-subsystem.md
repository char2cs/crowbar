# Crowbar Backend — Git Subsystem

> **Status:** Approved
> **Date:** 2026-06-03
> **Depends on:** `00-architecture-and-domain.md`, `02-api-surface.md`,
> `03-realtime-websockets.md`
> **Scope:** The git execution layer — read ops, write ops, hunk-level staging,
> conflict resolution. Covers UX spec §8, §9, §10, §22, §24, and the blame
> portion of §7.

---

## 1. Execution Strategy — Shell Out to `git`

The engine **shells out to the system `git` binary** (not a pure-Go library).
Every operation execs `git ...` in the workspace's repo directory and parses the
output.

Rationale: Crowbar is a local developer tool; the user has `git` installed and
configured. The real binary is 100% compatible with the user's config, hooks,
and credential setup, and reliably supports the advanced operations the UX spec
demands (hunk staging via `git apply --cached`, rebase, stash, three-way
conflict inspection) that pure-Go libraries implement incompletely.

### Credentials

Push / pull / fetch **rely entirely on the user's existing git credential
setup** — SSH agent, `git credential` helper, `.gitconfig`. The backend manages
**no credentials of its own**: it shells out, and whatever the user's git is
configured to do, happens. No tokens stored, no auth prompts handled by Crowbar.

---

## 2. Package Structure

```
internal/engine/git/                  git execution engine (engine layer)
  git.go              GitEngine interface — all operations
  internal/
    exec/exec.go      run git in a workdir; capture stdout/stderr/exit code
    status/           parse porcelain=v2 → GitStatus
    diff/             parse unified diff → FileDiff / MultiFileDiff; assign hunkId
    log/              parse git log → Commit[]
    blame/            parse git blame --porcelain → BlameEntry[]
    branches/         branch list / create / rename / delete / checkout
    stash/            stash list / push / apply / pop / drop
    conflicts/        detect conflicts; parse markers + stages → ConflictHunk[]
    apply/            hunkId → reconstructed patch fragment → git apply --cached
```

The engine is **stateless per call** — every method receives the repo path (and
branch where relevant) as context. It lives in the `engine/` layer (like
quiver.core engines) and is consumed by `app/usecases/git.go`. The usecase is
where broadcasts and error classification happen.

---

## 3. Read Operations

| Operation | git command | Returns |
|-----------|-------------|---------|
| Status | `git status --porcelain=v2 --branch` | `GitStatus { branch, ahead, behind, files[] }` — **unmerged (`u`) records → `status: conflicted`** so the Changes panel can badge conflicting files and route a click into conflict mode (UX §24); do not drop the `u` records |
| Log    | `git log --skip=N --max-count=50 --pretty=<fmt>` | `Commit[]` (paginated) |
| Diff (working tree) | `git diff -M` / `git diff -M --cached` (`-M` so renames are detected as `is_renamed` rather than add+delete, regardless of the user's `diff.renames` config) | `FileDiff` with `hunkId` per hunk |
| Diff (commit) | `git diff -M <sha>^ <sha>` — for the **root commit** (no parent) fall back to `git show --format= <sha>` (or `git diff --root <sha>`) | `MultiFileDiff` |
| Branches | `git branch -a --format=<fmt>` | `Branch[]` |
| Stashes | `git stash list` | `Stash[]` |
| Blame | `git blame --porcelain <file>` | `BlameEntry[]` |

**Pagination** (log): `limit` / `skip` query params map to `--max-count` /
`--skip`. Default 50. The log is `HEAD`'s history (full ancestry of the
workspace's branch, UX §9) — not scoped to `<base>..HEAD`; the History tab shows
the complete commit history reachable from the branch, like any git client.

**Blame** annotates each line of a file with the commit that last changed it
(author, date, message). It powers the editor's inline blame (UX §7). It is a
git read op implemented in `internal/blame/` and exposed at the editor-friendly
URL `GET /v0/workspaces/:wsId/blame` — the endpoint location is an API
convenience; the implementation belongs to git.

**Binary / image diff blobs.** `git diff` only reports "Binary files differ" for
binary paths — it does not emit bytes. So for `is_binary` / `is_image` files the
`diff/` package makes **separate `git show <blob>` / `git cat-file blob` calls per
side** to populate `old_blob_base64` / `new_blob_base64` (UX §10 image diff, §31).
The blob SHAs come from the diff's raw header (`git diff --raw` /
`:<mode> <mode> <sha> <sha>`). Text diffs never trigger this path.

### Data shapes (from UX spec)

```
GitStatus { branch, ahead, behind, files: GitFile[] }
GitFile   { path, status: modified|added|deleted|untracked|renamed|conflicted, staged: bool }
Commit    { hash, shortHash, message, description?, author, email?, date }
Branch    { name, isCurrent, isRemote, ahead?, behind?, lastCommitDate? }
Stash     { id, message, date, filesChanged }
BlameEntry{ lineNumber, commitHash, author, email, date, commitMessage }
FileDiff  { file_path, old_path?, new_path?, is_new, is_deleted, is_renamed,
            is_binary?, is_image?, old_blob_base64?, new_blob_base64?,
            lines: DiffLine[], additions, deletions, hunks: Hunk[] }
DiffLine  { line_type: added|removed|context|header, content,
            old_line_number?, new_line_number?, hunkId? }   // hunkId set on changed lines
Hunk      { hunkId, header, startLine, endLine }            // line range this hunk covers in lines[]
MultiFileDiff { commitHash?, commitMessage?, commitDescription?, commitAuthor?,
                commitDate?, files: FileDiff[], totalFiles, totalAdditions,
                totalDeletions }
```

---

## 4. Hunk-Level Staging

A **hunk** is a contiguous block of changed lines within one file's diff. The
UX (§8, §10) lets the user stage/unstage individual hunks.

`FileDiff` carries **both** representations of the same diff: `lines[]` (the flat
list the UX renders) and `hunks[]` (the staging units). They are linked by
`hunkId` — each `Hunk` records the `[startLine, endLine]` range it covers in
`lines[]`, and each changed `DiffLine` carries its `hunkId`. So the frontend
renders from `lines[]` and draws a per-hunk "stage" button using the `hunks[]`
ranges, sending `{ path, hunkId }` to stage (§ below). Context/header lines have
no `hunkId`.

### Hunk ID assignment

When the `diff/` parser produces a hunk it computes:

```
hunkId = sha256(filePath + "\n" + hunkBody)[:12]     // body = the +/-/context
                                                     // lines only; NOT the @@ header
```

The hash deliberately **excludes the `@@` header**, because the header carries
line numbers that **shift when a *sibling* hunk in the same file is staged** —
hashing the header would change later hunks' ids mid-workflow and break "stage
hunk A, then hunk B" with a spurious "hunk not found." Hashing only the body (the
actual changed + context lines) keeps the id stable across sibling staging. The
id is **embedded in the `FileDiff`** returned to the frontend (each hunk carries
its `hunkId`). On a stage, the backend re-diffs and matches by recomputed
body-hash, so it locates the right hunk even after earlier hunks moved.

**Stale-hunk contract.** Body-only hashing survives *sibling* staging, but **not a
content edit** to the same lines between fetching the diff and clicking stage
(the normal editor flow — the file is being actively edited). If no current hunk's
recomputed `hunkId` matches the requested one, the stage returns a **`stale_hunk`**
error (§8); the frontend re-fetches the diff and retries. This makes the failure
explicit rather than a silent mis-stage.

### Staging a hunk

The frontend sends `{ path, hunkId }` (never raw patch text). The `apply/`
package:

1. Runs a fresh `git diff` for `path` (normal context, e.g. `-U3`).
2. Locates the hunk whose recomputed `hunkId` matches.
3. Reconstructs the minimal patch (file header + that single `@@` hunk **with its
   context lines intact**).
4. Pipes it to `git apply --cached` (or `--cached -R` to unstage).

> **No `--unidiff-zero`.** That flag is for **zero-context** patches; applying it
> to a normal hunk that carries context lines (the kind plain `git diff` emits)
> mis-applies or rejects. We keep the hunk's context and apply with plain
> `git apply --cached`, whose context matching places the hunk correctly even when
> other hunks in the file are unstaged.

Git-patch knowledge stays entirely on the backend; the frontend only ever
references hunks by id. (Sub-hunk / line-level staging is **not** required by the
UX spec and is out of scope.)

---

## 5. Write Operations

All map to a git command and run through the same `exec/` runner. Every op that
mutates the working tree or index triggers a `GitStatus` recompute and broadcast
(see §7).

> **Locked guard.** A `locked` (provider-protected) workspace is chat-only
> (`07` §2). Every write op here — and every file write in
> `05-filesystem-and-watcher.md` §3/§4 — first checks the workspace and **rejects
> with `locked` error** if it is locked. The only thing permitted on a locked
> workspace is chat creation.

| Op | Command |
|----|---------|
| Stage (files) | `git add <paths>` |
| Stage (hunk) | `git apply --cached` with reconstructed patch (§4) |
| Unstage | `git restore --staged <paths>` |
| Discard | `git restore <paths>` for tracked-modified; `git clean -f <paths>` for **untracked** files (UX §8 "discard" expects an untracked file to be removed — `git restore` alone is a silent no-op on it). Behind the §8 confirmation dialog. |
| Commit | `git commit -m <subject> [-m <body>]` |
| Push / Fetch | `git push` / `git fetch` |
| Pull | `git pull --no-rebase` (merge) or `git pull --rebase` per the request's `{ mode: "merge"\|"rebase" }` (UX §22) — **not** bare `git pull`, which would nondeterministically honor the user's `pull.rebase` config. A conflicted pull finalizes/aborts via §6.1. |
| Create branch | `git checkout -b <name> [<source>]` (creates **and** switches; pass `checkout:false` → `git branch <name>` to create without switching) |
| Rename branch | `git branch -m <old> <new>` |
| Delete branch (user-facing) | `git branch -d <name>` (blocked if current; `-d` surfaces "not fully merged" so the user can confirm). Workspace **teardown** instead uses `-D` — `07` §5 |
| Switch branch | `git checkout <branch>` |
| Stash | `git stash push [-m <msg>]` |
| Stash apply/pop | `git stash apply <id>` / `git stash pop <id>` |
| Stash drop | `git stash drop <id>` |
| Reset | `git reset --soft\|--mixed\|--hard <commit>` |
| Merge | `git merge <branch>` (generic, single-worktree — see note) |
| Rebase | `git rebase <onto>` (generic, single-worktree — see note) |

**Switch branch — reconciled with the worktree model.** A workspace *is* a
`git worktree` pinned to one branch, and git refuses to check out a branch already
checked out in another worktree. So "Switch branch" (UX §22) has two cases:

- Target branch **is already a workspace** (materialized in a sibling worktree):
  do **not** `git checkout` — instead **navigate to that sibling workspace**. This
  is the common case in Crowbar's model and is a client-side navigation, not a git
  op.
- Target branch is **not materialized** as any workspace: run
  `git checkout <branch>` in this worktree, re-resolve the workspace's `branch`,
  refresh the file tree, and re-broadcast status. (If the checkout fails because
  the branch is in use elsewhere, fall back to the navigate case.)

Switching changes the working tree; the usecase re-resolves `branch`, refreshes
the file tree, and re-broadcasts. Two guards:
- **Locked workspace:** switch-branch is **rejected** on a `locked` workspace —
  re-pointing a protected workspace at another branch is a mutation it doesn't
  permit (`07` §2).
- **Nonexistent target:** "switch" operates on **existing** branches only.
  Creating a branch is the separate **Create branch** op (`git checkout -b`,
  above); the switch entry point does not create.

### Generic `merge` / `rebase` vs. `merge-into-parent`

The generic git-write `merge {branch}` / `rebase {onto}` ops (UX §22) act on the
**current workspace's own worktree** — distinct from the cross-worktree
`merge-into-parent` (`07` §3.1). They do **not** use the `pendingMerge` marker; a
conflict leaves the ordinary on-disk marker (`MERGE_HEAD` / `rebase-merge/`) and
is finalized/aborted via the single-worktree path of §6.1. Like switch-branch, the
`<branch>` / `<onto>` target **must not be checked out in a sibling worktree**
(git refuses); if it is, the op is rejected with guidance to operate from that
sibling workspace.

### Per-repo serialization (concurrency)

All worktrees of a repo **share one `.git` object store, refs, and
`index.lock`**. A `git merge` in one worktree while a `git rebase` runs in a
sibling worktree (or a `SyncWorkingTreeState` status recompute reads refs
mid-rewrite) will contend and fail with a lock error. The usecase therefore holds
a **per-repository (not per-worktree) mutation lock** around every write op and
around the re-parent rebase (`07` §4). The lock is keyed by `repoId`, not `wsId`.

A multi-step rewrite (merge/rebase) leaves the repo in a transiently
**inconsistent** state (detached/rewritten HEAD, partial index), not merely
locked — a status recompute that ran then could emit a nonsense `+N/-N` or a
spurious `hasConflicts`. So a `SyncWorkingTreeState` recompute **must not run
while a rewrite is in progress** for that repo: the watcher checks for an
in-progress operation (`.git/MERGE_HEAD`, `rebase-merge/`, `rebase-apply/`) and
**defers** the recompute until it clears, rather than reading a half-rewritten
state. (A genuine mid-merge `hasConflicts:true` is set explicitly by the conflict
flow, `04` §6 — not inferred from a transient snapshot.) The provider poller's
read-only `gh`/`glab` calls don't touch `.git` and are unaffected.

---

## 6. Conflict Resolution — UX §24

When merge / rebase / pull exits with conflicts:

1. The engine detects it: non-zero exit **and** `git status` reports unmerged
   paths.
2. The usecase issues `SyncWorkingTreeState{hasConflicts: true, …}` to the
   Workspace aggregate (recomputed from git — `git status` unmerged paths for
   `hasConflicts`, per `00` §5.3) → the aggregate projects and broadcasts on the
   Workspaces topic (the sidebar shows the conflict indicator). It does **not** set
   `hasConflicts` out of band — same single command as the watcher (`00` §5.3,
   §6.1).
3. `conflicts/` builds the three-way view for each conflicting file:
   - Working-tree markers (`<<<<<<<`, `=======`, `>>>>>>>`) delimit hunks.
   - `git show :1:<path>` = **base** (common ancestor),
     `:2:<path>` = **ours**, `:3:<path>` = **theirs**.
   - Produces `ConflictHunk[]`.
4. The frontend resolves per hunk;
   `POST /v0/workspaces/:wsId/git/conflicts/resolve
   { path, conflictHunkId, resolution, resolvedContent? }` writes the resolved
   content into the file. `conflictHunkId` is `ConflictHunk.id` (below) — **not**
   the staging `hunkId` of §4. **When a file has no remaining conflict markers,
   the resolve usecase runs `git add <path>`** to clear its unmerged index
   entries — without this stage step the index stays unmerged and
   `operation/continue` (§6.1) would fail (`git commit` / `rebase --continue`
   refuse with unmerged paths).
5. When every hunk in every conflicting file is resolved and staged, the user
   **completes** the operation (§6.1) — resolving hunks alone does **not** finish
   the merge/rebase; an explicit finalize step does. On completion the usecase
   issues `SyncWorkingTreeState{hasConflicts: false, …}` → broadcast.

### 6.1 Completing or aborting an in-progress operation

A conflicted `merge` / `rebase` / `pull` (and the `merge-into-parent` rebase
strategy, `07` §3.1) leaves the repo in an **in-progress** state with a marker on
disk (`.git/MERGE_HEAD`, `rebase-merge/`, `rebase-apply/`). Resolving hunks stages
content but does **not** finalize — the operation must be explicitly continued or
aborted. UX §24 has both a "complete the merge commit" action and an abort.

```
POST /v0/workspaces/:wsId/git/operation/continue   finalize the in-progress op
POST /v0/workspaces/:wsId/git/operation/abort       roll back to pre-op state
```

The usecase reads the on-disk marker to know which operation is in progress and
runs the right finalize:

| In progress | Continue | Abort |
|-------------|----------|-------|
| merge | `git commit` (pre-populated message) | `git merge --abort` |
| squash | `git commit` | `git merge --abort` / `git reset --merge` |
| rebase / pull-rebase | `git rebase --continue` (loops if more conflicted commits) | `git rebase --abort` |
| pull-merge | `git commit` | `git merge --abort` |

**`merge-into-parent` spans two worktrees**, so its in-progress state can't be
inferred from one worktree's marker. The Workspace aggregate records a
**`pendingMerge { strategy, targetParentId }`** marker when `merge-into-parent`
starts and conflicts. `operation/continue` then drives the strategy to completion
— for the `rebase` strategy: finish the child rebase (`git rebase --continue`)
**then** run `git merge --ff-only` in the parent worktree (one locked critical
section), update `forkPointSha` (`07` §3.1), and clear `pendingMerge`. `abort`
runs `git rebase --abort` in the child and clears the marker (the parent was never
advanced). After a clean finalize, `hasConflicts` clears.

> The single-locked-critical-section guarantee holds for an **unconflicted**
> rebase merge. For a **conflicted** one, resolution happens across separate HTTP
> requests, so the parent could (in principle) advance between the child rebase
> and the resume — then `--ff-only` fails. The usecase falls back by re-running
> `git rebase --continue` semantics against the new parent tip (or surfaces a
> `dirty_tree`/`rejected_non_fast_forward` error to retry). In a single-user local tool
> this race is rare but is handled, not assumed away.

### Shape (from UX spec)

```
ConflictHunk {
  id, startLine, endLine,
  ours, theirs, base?,
  resolution: ours|theirs|both|custom|unresolved,
  resolvedContent?      // set when resolution = custom
}
```

`resolution: both` concatenates ours+theirs. `id` here is the conflict hunk id
within the file (distinct from the staging `hunkId` of §4).

---

## 7. Real-time Integration

The git subsystem is both a **producer** and a **consumer** of real-time
signals (see `03-realtime-websockets.md`):

- **File watcher → git** (Class B): every disk write recomputes `GitStatus` and
  re-broadcasts **directly** on the Git topic (`wsId`). The workspace's `+N/-N`
  diff stats and `hasConflicts` go **through the Workspace aggregate** via a
  `SyncWorkingTreeState` command (Class A) so the global Workspaces row stays a
  single complete object — not a direct broadcast (`03` §2, §5).
- **Git write ops → broadcast**: any mutation (commit, stage, checkout, merge…)
  recomputes status, pushes `GitStatus`, and issues `SyncWorkingTreeState`, so the
  panel and row reflect the change without the frontend re-fetching.

`GitStatus` push is therefore driven by **both** the filesystem watcher and
explicit git mutations — never by polling.

---

## 8. Error Handling

Git failures return a structured error through the standard envelope:

```
{ success: false, error: "<git stderr>", code: "<classified>" }
```

The usecase classifies common cases so the frontend can show the correct dialog:

| Code | Cause |
|------|-------|
| `conflict` | merge/rebase/pull produced conflicts |
| `rejected_non_fast_forward` | push rejected (remote ahead) |
| `nothing_to_commit` | commit with empty index |
| `dirty_tree` | operation blocked by uncommitted changes |
| `stale_hunk` | hunk-stage `hunkId` no longer matches (file edited since the diff was fetched) — frontend re-fetches and retries (§4) |
| `has_children` | re-parent / rebase-strategy merge rejected because the node has descendants (`07` §4 / §4.1) |
| `auth_failed` | credential failure (surfaced from git, not handled by us) |
| `unknown` | anything else; raw stderr passed through |

---

## 9. Out of Scope

- Pure-Go git (rejected — see §1).
- Credential management (rejected — user's own git credentials, §1).
- Sub-hunk / line-level staging (not in UX spec).
