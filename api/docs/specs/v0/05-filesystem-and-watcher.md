# Crowbar Backend — Filesystem & File Watcher

> **Status:** Approved
> **Date:** 2026-06-03
> **Depends on:** `00-architecture-and-domain.md`, `02-api-surface.md`,
> `03-realtime-websockets.md`, `04-git-subsystem.md`
> **Scope:** File tree, file content read/write, structural mutations, and the
> live disk watcher. Covers UX spec §6, §7 (content), §21, §27, §31.

---

## 1. Two Halves

The subsystem splits into a **request/response half** (read tree, read/write
content, create/rename/delete) and a **push half** (the watcher that detects
external changes). Both share a workspace's repo root path as context.

```
internal/engine/fs/
  fs.go               FSEngine interface
  internal/
    tree/             walk dir → FileNode[], merge git status decorations
    content/          read/write file bytes; detect text vs binary, encoding
    mutate/           create / rename / move / delete (file + folder)
    watch/            fsnotify wrapper → FileChangeEvent, debounced
```

The engine is stateless per call (like the git engine); the watcher is the one
stateful, per-workspace, lazily-started resource.

---

## 2. File Tree — UX §6

```
GET /v0/workspaces/:wsId/files/tree?path=
```

- **Lazy loading.** Returns **one level** per call. Expanding a directory calls
  again with `?path=<subdir>`. This avoids walking huge trees (e.g.
  `node_modules`) up front. The UX `FileNode.children?` being optional supports
  this directly.
- **Git decorations.** The walker calls the git engine's status once and merges
  `gitStatus?` onto each node, so the file tree and the Git panel always agree.

```
FileNode { name, path, type: file|directory, children?, gitStatus? }
gitStatus = modified | added | deleted | untracked | renamed
```

### Ignore rules (Q1 — IDE behavior)

The tree and the watcher behave like common IDEs:

- Always skip `.git/`.
- Respect `.gitignore` for **watching** — ignored paths produce no watch events
  (so we don't drown in `node_modules` churn or exhaust inotify limits).
- The tree **may still display** ignored files (greyed/decorated), but they are
  not watched. Displaying-vs-watching are independent concerns.

---

## 3. File Content — UX §7, §31

### Read

```
GET /v0/workspaces/:wsId/files/content?path=
→ FileContent { content: string, encoding?: string }   // 'base64' for binary
```

`content/` sniffs text vs binary (null-byte scan / UTF-8 validity). Text is
returned UTF-8; binary is base64 with `encoding: 'base64'`.

**One endpoint serves everything that needs file bytes** — the Monaco editor,
all five preview panes (markdown / html / csv / image / pdf, UX §31), and image
diffs. Rendering is entirely client-side; the backend only serves bytes.

### Write

```
PUT /v0/workspaces/:wsId/files/content { path, content }
```

Writes UTF-8 to disk. The **most frequent write in the app**. It writes bytes
only — formatter / linter on save are client- or LSP-side concerns, not this
endpoint's job. The write triggers the watcher, which fans out (§5).

---

## 4. Structural Mutations — UX §21

| Op | Endpoint | Action |
|----|----------|--------|
| New file / folder | `POST /v0/workspaces/:wsId/files { path, type }` | create empty file or `mkdir` |
| Rename / move | `PATCH /v0/workspaces/:wsId/files { path, newPath }` | `os.Rename` |
| Delete | `DELETE /v0/workspaces/:wsId/files { path }` | remove file / dir |

All are plain filesystem ops in `mutate/`. Each triggers the watcher →
broadcasts. The frontend handles the UI consequences client-side by watching the
`FileChangeEvent` stream:

- After **rename/move**: re-point any open tabs referencing the old path.
- After **delete**: close any open tab for the removed path.

The backend does **not** track which files are "open."

---

## 5. The Watcher — UX §27

Per-workspace, **lazily started on first WS subscription** (Section 4, Q1 = A),
built on `fsnotify`.

### Recursive watching (Q2)

`fsnotify` has no native recursive watch. The `watch/` package **manages
recursion itself**: walk the tree and add a watch per directory, then add/remove
watches as directories appear/disappear. Standard approach, minimal deps.

### Pipeline

```
watch/ watches the repo root (recursively, honoring ignore rules §2)
  → raw fsnotify event
  → debounce (coalesce bursts, ~100ms window)
  → classify: created | modified | deleted | renamed
  → hub.BroadcastFile(FileChangeEvent)       → Files topic       (wsId)
  → recompute GitStatus
       → hub.BroadcastGit(GitStatus)          → Git topic         (wsId)
  → recompute +N/-N (and hasConflicts)
       → hub.BroadcastWorkspace(Workspace)    → Workspaces topic  (global)
```

```
FileChangeEvent { type: created|modified|deleted|renamed, path, newPath? }
```

This single fan-out (one disk event → three topics) is what makes agent work
visible live: agent writes a file → watcher fires → editor tab reloads, file
tree updates, git panel refreshes — with no user action (UX §27).

Debouncing bounds the broadcast rate when an agent edits many files in a burst.

---

## 6. External Change vs. Unsaved Edits — UX §20 (Q3)

The "file changed on disk while open with unsaved edits → prompt reload/keep"
behavior is **client-side**. The backend's only responsibilities are:

- Emit the `FileChangeEvent` (it already does, §5).
- Serve current bytes on request (`GET .../files/content`).

The frontend compares its dirty buffer against the event and shows the
reload/keep prompt. The backend tracks no "open" or "dirty" state.

---

## 7. Real-time Integration

The watcher is the busiest **Class B producer** (`03-realtime-websockets.md`).
It is the single source that drives the Files topic and, through recomputation,
the Git and Workspaces topics. The watcher and the git status recompute share
the same per-workspace lifecycle (lazy start / ref-counted teardown across the
Files, Git, and LSP subscriptions for that `wsId`).

---

## 8. Out of Scope

- Formatter / linter execution on save (client or LSP).
- Tracking open/dirty buffers (client-side, UX §18).
- Recent files, pane layout (client-side IndexedDB, UX §18).
```
