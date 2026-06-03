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
| Status | `git status --porcelain=v2 --branch` | `GitStatus { branch, ahead, behind, files[] }` |
| Log    | `git log --skip=N --max-count=50 --pretty=<fmt>` | `Commit[]` (paginated) |
| Diff (working tree) | `git diff` / `git diff --cached` | `FileDiff` with `hunkId` per hunk |
| Diff (commit) | `git diff <sha>^ <sha>` | `MultiFileDiff` |
| Branches | `git branch -a --format=<fmt>` | `Branch[]` |
| Stashes | `git stash list` | `Stash[]` |
| Blame | `git blame --porcelain <file>` | `BlameEntry[]` |

**Pagination** (log): `limit` / `skip` query params map to `--max-count` /
`--skip`. Default 50.

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
GitFile   { path, status: modified|added|deleted|untracked|renamed, staged: bool }
Commit    { hash, shortHash, message, description?, author, email?, date }
Branch    { name, isCurrent, isRemote, ahead?, behind?, lastCommitDate? }
Stash     { id, message, date, filesChanged }
BlameEntry{ lineNumber, commitHash, author, email, date, commitMessage }
FileDiff  { file_path, old_path?, new_path?, is_new, is_deleted, is_renamed,
            is_binary?, is_image?, old_blob_base64?, new_blob_base64?,
            lines: DiffLine[], additions, deletions, hunks: Hunk[] }
DiffLine  { line_type: added|removed|context|header, content,
            old_line_number?, new_line_number? }
MultiFileDiff { commitHash?, commitMessage?, commitDescription?, commitAuthor?,
                commitDate?, files: FileDiff[], totalFiles, totalAdditions,
                totalDeletions }
```

---

## 4. Hunk-Level Staging

A **hunk** is a contiguous block of changed lines within one file's diff. The
UX (§8, §10) lets the user stage/unstage individual hunks.

### Hunk ID assignment

When the `diff/` parser produces a hunk it computes:

```
hunkId = sha256(filePath + "@@" + hunkHeader + hunkBody)[:12]
```

This is **stable** while the hunk's content is unchanged, and is **embedded in
the `FileDiff`** returned to the frontend (each hunk carries its `hunkId`).

### Staging a hunk

The frontend sends `{ path, hunkId }` (never raw patch text). The `apply/`
package:

1. Runs a fresh `git diff` for `path`.
2. Locates the hunk whose recomputed `hunkId` matches.
3. Reconstructs the minimal patch (file header + that single `@@` hunk).
4. Pipes it to `git apply --cached --unidiff-zero` (or `-R` to unstage).

Git-patch knowledge stays entirely on the backend; the frontend only ever
references hunks by id. (Sub-hunk / line-level staging is **not** required by the
UX spec and is out of scope.)

---

## 5. Write Operations

All map to a git command and run through the same `exec/` runner. Every op that
mutates the working tree or index triggers a `GitStatus` recompute and broadcast
(see §7).

| Op | Command |
|----|---------|
| Stage (files) | `git add <paths>` |
| Stage (hunk) | `git apply --cached` with reconstructed patch (§4) |
| Unstage | `git restore --staged <paths>` |
| Discard | `git restore <paths>` |
| Commit | `git commit -m <subject> [-m <body>]` |
| Push / Pull / Fetch | `git push` / `git pull` / `git fetch` |
| Create branch | `git checkout -b <name> [<source>]` (creates **and** switches; pass `checkout:false` → `git branch <name>` to create without switching) |
| Rename branch | `git branch -m <old> <new>` |
| Delete branch | `git branch -d <name>` (blocked if current) |
| Switch branch | `git checkout <branch>` |
| Stash | `git stash push [-m <msg>]` |
| Stash apply/pop | `git stash apply <id>` / `git stash pop <id>` |
| Stash drop | `git stash drop <id>` |
| Reset | `git reset --soft|--mixed|--hard <commit>` |
| Merge | `git merge <branch>` |
| Rebase | `git rebase <onto>` |

Switching branches changes the workspace's working tree; the usecase
re-resolves the workspace's `branch`, refreshes the file tree, and re-broadcasts
status.

### Per-repo serialization (concurrency)

All worktrees of a repo **share one `.git` object store, refs, and
`index.lock`**. A `git merge` in one worktree while a `git rebase` runs in a
sibling worktree (or a `SyncWorkingTreeState` status recompute reads refs
mid-rewrite) will contend and fail with a lock error. The usecase therefore holds
a **per-repository (not per-worktree) mutation lock** around every write op and
around the re-parent rebase (`07` §4). Read-only ops (status recompute, the
provider poller's `gh pr view`) do not take the write lock but must tolerate a
transient mid-rewrite ref state (retry on `index.lock`). The lock is keyed by
`repoId`, not `wsId`.

---

## 6. Conflict Resolution — UX §24

When merge / rebase / pull exits with conflicts:

1. The engine detects it: non-zero exit **and** `git status` reports unmerged
   paths.
2. The usecase sets `hasConflicts: true` on the Workspace aggregate → broadcast
   on the Workspaces topic (the sidebar shows the conflict indicator).
3. `conflicts/` builds the three-way view for each conflicting file:
   - Working-tree markers (`<<<<<<<`, `=======`, `>>>>>>>`) delimit hunks.
   - `git show :1:<path>` = **base** (common ancestor),
     `:2:<path>` = **ours**, `:3:<path>` = **theirs**.
   - Produces `ConflictHunk[]`.
4. The frontend resolves per hunk;
   `POST /v0/workspaces/:wsId/git/conflicts/resolve
   { path, conflictHunkId, resolution, resolvedContent? }` writes the resolved
   content into the file. `conflictHunkId` is `ConflictHunk.id` (below) — **not**
   the staging `hunkId` of §4.
5. When every hunk in every conflicting file is resolved and staged,
   `hasConflicts` clears → broadcast.

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
| `auth_failed` | credential failure (surfaced from git, not handled by us) |
| `unknown` | anything else; raw stderr passed through |

---

## 9. Out of Scope

- Pure-Go git (rejected — see §1).
- Credential management (rejected — user's own git credentials, §1).
- Sub-hunk / line-level staging (not in UX spec).
