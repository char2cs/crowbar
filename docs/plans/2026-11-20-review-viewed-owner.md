# Review "viewed" map: the native shell owns it

Status: approved 2026-11-20 (coordinator). Rows: SB-230, SB-234, SB-235, and the reveal half of SB-237.

## Why
Develop keeps the viewed map in the web app's IndexedDB (`web/src/lib/persistence/branch-review-viewed.ts`,
store `branch-review`, key `wsId`, value `{ wsId, viewed: { [path]: signature } }`). The Changes
tree and the review pane share it because they run in one webview. In the Rust shell the Changes
tree is native and the review pane is a web page with its own IndexedDB. One of them has to own
the map. The owner is the native shell, for two reasons:
- The native Changes tree has to show its ticks while no review pane is mounted, so the pane
  cannot own the map.
- The shell is always alive and already persists its own state.

There is no second copy and no daemon endpoint.

## Owner and persistence
- **Owner:** the `store` crate. A new held type `live/held/viewed_files.rs`
  (`ViewedFiles: BTreeMap<workspace id, BTreeMap<path, signature>>`) sits next to `PanelState`
  and is not reset when the followed chat changes.
- **Persistence:** `<state_dir>/review-viewed.json`, beside `layout.json`, `sidebar.json` and
  `window.json`. It is read and written through the store's existing `Files` port
  (`live/files.rs`, whose disk adapter writes through `atomicfile`), so tests use the port's
  in-memory fake and no new adapter is needed. The shape is develop's, keyed per workspace: `{ "<wsId>": { "<path>": "<signature>" } }`.
  A file that is missing or unreadable reads as empty and is logged; it is never fatal.
- **Signature:** the same string as develop's `fileSignature`:
  `"{kind}|{old_path or empty}|{additions}|{deletions}"`. `kind` is `A` for added or untracked,
  `D` for deleted, `R` for renamed, otherwise `M`, taken from the branch summary
  (`GET chats/:id/review/files`, as develop's `reviewFileSummaryToGitDiff` does).
  - `sidebar::ChangedFile` gains `signature: String`, filled by the loader from the summary's raw
    counts and `old_path` through `ChangedFile::signature_of(status, old_path, counts)`. The
    displayed counts are clamped to `u32`, so the signature has to be taken before clamping.
  - The summary's counts become `i64`: binaries come back as `-1`, and develop's signature keeps
    the `-1`. Today `SummaryFile` reads them as `u32`, so a summary with a binary fails to parse,
    and that fix comes first.
- **Writes:** a new `sidebar::Command::SetFilesViewed { files: Vec<(String, Option<String>)> }` for
  the followed workspace: each path with the signature to mark it with, or `None` to clear it (no
  bool flag). The store applies it, persists it and reports `Outcome::ReviewViewed { workspace,
  viewed }`. The pane's write goes through the same call, `SidebarCommands::set_review_viewed(chat,
  changes)`, and the shell reads a workspace's map with `SidebarCommands::review_viewed(chat)`, so
  both surfaces write through one owner.
- **Reads:** `reads::changes` puts the followed workspace's map into `Changes.viewed`.
  `Changes::rows` gives each file `viewed` (its stored signature equals the current one) and each
  folder `viewed/total`. The sidebar draws develop's checkbox (`ui::Checkbox`, with indeterminate
  for a partly viewed folder) in the 22px control slot. A viewed name drops its status colour.

## Protocol (`protocol` crate, SDK frames)
- `PaneFrame::ReviewSetViewed` (`"review.setViewed"`): `{ files: [{ path, signature }], viewed }`.
  It names no workspace: the shell takes it from the pane's own chat.
  Fire-and-forget, like `settings.set`. The shell stays the owner, and the pane sees the result
  when the shell pushes the map again.
- `ShellFrame::ReviewViewed` (`"review.viewed"`): `{ workspace, viewed: { path: signature } }`.
  The shell sends it to a review pane right after `mount` (alongside the context frames) and to
  every mounted `core.review` pane of that workspace whenever the map changes, from either
  surface.
- `ShellFrame::ReviewReveal` (`"review.reveal"`): `{ path }`. It is sent to one review pane when a
  Changes file is clicked and that pane is already open and mounted. A review opened by the click,
  or an open one whose page is not mounted (a tab never shown), gets `{ "reveal": path }` in its
  saved state instead, so the reveal applies on mount and is never lost.
- Files: `app/crates/protocol/src/control/pane_frame.rs`, `shell_frame.rs` and one payload type
  `control/payload/viewed_file.rs`.

## panehost (lane P3 works here, so the edits stay small)
- `pane_host/lifecycle/frames.rs`: `PaneFrame::ReviewSetViewed` becomes
  `HostEvent::ReviewViewedSet { pane, files, viewed }`. A page that registers and mounts is
  reported as `HostEvent::Mounted { pane }`, so the shell can send the map once the page listens.
- `slot/inbox/host_event.rs`: those two variants.
- No `send_to_type` or other broadcast helper: the public `PaneHost::send(pane, frame)` already
  exists, and only the shell knows which workspace each review pane's chat belongs to, so it picks
  the panes (`app/src/review_panes.rs`).
- Nothing in layoutview.

## app (the shell crate)
- `shell_state/host_events.rs` puts `HostEvent::ReviewViewedSet` and `Mounted` in the outcome;
  `shell_view/review_viewed.rs` hands the marks to `SidebarCommands::set_review_viewed`. The
  workspace comes from the pane's own `threadId`, never from the frame, so a pane cannot write
  another workspace's map.
- When the store reports a viewed change, the shell sends `review.viewed` to the review panes of
  that workspace. On a review pane's mount, it sends the map once.
- `Command::OpenReview` becomes `OpenReview { path }`. A review that is not open opens with
  `reveal` in its state. An open one is focused; if its page is mounted it is sent
  `review.reveal`, otherwise `reveal` is written into its saved state for the mount to apply.

## app/panes (one owner per process, never a fallback)
The mobile app loads `index.html` -> `main.tsx` -> `app-main`, the full app, which renders the
branch review (`review-code-view.tsx` uses `useReviewViewed`). So mobile has no native shell and
still shows the review. The slice therefore talks to an interface:
- `features/git/lib/viewed-owner.ts`: `ViewedOwner { follow(wsId, listener): () => void;
  set(wsId, change: ViewedChange): Promise<void> }`, where `ViewedChange` (`sdk/viewed-change.ts`)
  is `{ files, viewed }`. The owner is installed once at the entry point (`installViewedOwner`)
  and read by the slice through `viewedOwner()`.
- Desktop entries (`entries/review.tsx` and any other `entries/*` page that mounts the review)
  use `ShellViewedOwner`. `follow` first hears the last `review.viewed` frame, `set` posts
  `review.setViewed` and writes nothing locally, and `follow` hears later `review.viewed` frames.
- `app-main` (mobile and the plain web build) uses `IdbViewedOwner`: today's
  `branch-review-viewed.ts`, moved behind the interface. The IDB schema keeps the
  `branch-review` store. Desktop never opens it.
- Neither implementation ever falls back to the other.
- `branch-review-slice.ts` calls the injected owner in place of the IDB functions.
- `buffer-pane-mount`/`open-review-buffer`: on mount, a `reveal` in the saved state calls
  `setBranchReviewView(id, 'files')` and `revealBranchReviewFile(path)`, as develop's git panel
  does. A `review.reveal` frame does the same.

## Migration
The shell has never stored viewed state, so the map starts empty on first run: nothing shows as
viewed until the user ticks it. Viewed marks made in develop's Tauri IndexedDB are not imported.

## Tests (red first, through public APIs)
1. `sidebar`: `ChangedFile::signature` matches develop's strings for A/D/R/M, a rename with an old
   path, and a binary (`-1`).
2. `store`: a summary with a binary file (`additions: -1`) still fills the Changes list
   (it is red today: the whole body fails to parse).
3. `store`: `SetFilesViewed` marks and unmarks, survives a reload from the adapter (a
   memory-backed one in tests), and keeps other workspaces' maps.
4. `store`/`sidebar`: a file whose counts changed since it was marked reads as not viewed, and a
   folder reads `1/2` when half its files are viewed.
5. `sidebarview`: ticking a folder sends `SetFilesViewed` for every file under it, and unticking
   clears them all (SB-235).
6. `protocol`: the three frames round-trip as JSON in the shape the TS SDK sends.
7. `panehost`: `review.setViewed` from a pane becomes `HostEvent::ReviewViewedSet` for that pane,
   and a page that registers is reported `Mounted` (one of the wrong type is not).
8. `app`: a pane's `review.setViewed` is applied to the workspace of the pane's `threadId`, and the
   change is pushed to the review panes of that workspace only.
9. `app/panes` (vitest): with `ShellViewedOwner`, `setFilesViewed` posts `review.setViewed` and
   changes nothing locally, and a `review.viewed` frame sets `viewedFiles` and `viewedLoaded`.
   With `IdbViewedOwner`, today's IDB behaviour holds (the existing tests move to it).
