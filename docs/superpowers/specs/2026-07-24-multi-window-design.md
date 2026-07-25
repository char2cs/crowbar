# Multi-Window Crowbar — Design Spec

**Date:** 2026-07-24
**Status:** Approved for implementation
**Branch:** `enhancement/restyling`

## Problem

A Crowbar window shows exactly one workspace at a time. Agent chats run per
workspace, so watching two workspaces' agents means switching back and forth —
you can never see both at once. The chats keep running across a switch (terminal
session persistence, Phase 1), so the state is there; it is only ever *visible*
one workspace at a time.

## Goal

Let the user open a second, identical Crowbar window pointed at a different
workspace, sharing the one daemon, so two workspaces' agent chats are on screen
simultaneously.

"Identical" is literal: the new window is the same app with the same sidebar,
the same everything. It is not a stripped-down chat viewer.

## Why duplicate windows, and not cross-workspace panes

A pane inside one window showing another workspace's chat was considered and
rejected. The pane/buffer system hangs off a single active workspace:
`_activeWorkspaceId` in `web/src/features/workspace/stores/workspace-store-registry.ts:12`,
plus `setWorkspaceScope` threading project+repo+workspace into every API and WS
URL. Cross-workspace panes means unpicking that.

Duplicating the window sidesteps all of it: each Tauri window is its own JS
realm, so it gets its own store registry, its own active workspace, and its own
scope map for free. The entire cost is confined to the Rust layer, where three
pieces of app-global state currently assume one window.

## What already works (verified by reading, not assumed)

These are load-bearing — the small footprint depends on them.

1. **The daemon is already a multi-client server.** `WatcherManager`
   (`api/internal/app/realtime/watcher_manager.go:39`) is refcounted per `wsId`
   with a linger timer: two workspaces means two independent watchers, the same
   workspace twice means `refs == 2`. Everything else fans out through the hub.
   There is no global "active workspace" anywhere in `api/`.
2. **WS connection ids are globally unique** — `crypto.randomUUID()` per JS realm
   (`web/src/lib/ws/tauri-transport.ts:46`). Two windows can never collide in
   `ws_bridge`'s map, so window-scoping it is a *tagging* change, not a
   *re-keying* change.

   > **Corrected during implementation: this does NOT extend to terminals.** The
   > original spec claimed terminal ids were unique too, "because they are
   > daemon-issued". Daemon-issued is precisely why they are *not* unique per
   > window: a terminal `connectionId` IS the daemon's PTY handle, and it is
   > deliberately shared between views — `attach-refcount.ts:14` ("two views on
   > the SAME PTY resolve to the SAME connectionId") and `agent-chat-pane.tsx`'s
   > `seedAttach`, which sets `connectionId = terminalSessionId`. Two windows on
   > the same workspace opening the same chat therefore collided: the second
   > window's `register` evicted the first's entry, and the evicted reader wakes
   > on the cancel branch, which is *silent by design* — no `transport-dropped`
   > fires, the frontend still believes its transport is live, and nothing
   > re-attaches. A permanently dead terminal with no error.
   >
   > `TerminalManager` is therefore keyed by **`(window_label, session_id)`** — a
   > real re-keying — so each window holds its own leg to the same PTY, which is
   > what the daemon's attach model already supports.
3. **Removal from the map is already the single teardown primitive** for both
   transports (documented at `desktop/src-tauri/src/ws_bridge.rs:33`). Scoping
   teardown to a window is therefore a `retain()` where there is a `clear()`.
4. **Per-webview wiring is already app-wide.** The `crowbar-bootstrap`
   `js_init_script` (`desktop/src-tauri/src/lib.rs:507`) and the `crowbar://`
   URI scheme apply to every webview, so a new window reaches the daemon with no
   extra work.
5. **Routing is hash-based** (`web/src/main.tsx:88`, `createHashHistory()`), so a
   window's target workspace is expressible purely as a URL fragment — no
   server-side routing, identical in dev and prod.
6. **Agent chat state is already workspace-scoped.** Chat row order is keyed
   `orderKey(wsId)` (`web/src/features/workspace/stores/slices/agent-chats-slice.ts:9`);
   chat PTYs are attached by daemon-issued `connectionId`; the attach refcount
   (`web/src/features/terminal/lib/attach-refcount.ts`) is a module-scoped map,
   i.e. per-window realm.
7. **`set_vibrancy_appearance`** (`desktop/src-tauri/src/lib.rs:390`) already
   takes the *calling* `WebviewWindow`, so it is multi-window-correct as written.

## What breaks with two windows

Four pieces of app-global Rust state. These are the whole problem.

| # | Site | Failure |
|---|---|---|
| 1 | `lib.rs:540-547` — `on_page_load` calls `WsBridgeManager::close_all()` and `TerminalManager::close_all()` | Window B loading or reloading kills **window A's** WebSockets and terminals |
| 2 | `lib.rs:604` — `on_window_event` tears down the sidecar on any window's close | Closing either window kills the daemon under the other |
| 3 | `terminal.rs:167` — `app.emit("terminal:transport-dropped", …)` | Window A reacts to window B's dropped transport |
| 4 | `lib.rs:566-590` — vibrancy + the WebKit 60fps un-cap applied only to `get_webview_window("main")` | A second window has no blur and is frame-capped |

## Design

### Window identity

The Tauri window label. The first window is `main` (from `tauri.conf.json`);
subsequent windows are `w<N>` from a process-lifetime `AtomicU64` counter,
never reused within a run.

### Transport scoping

The two transports are scoped **differently**, because their ids differ in kind
(see "What already works" #2 and its correction).

**`ws_bridge` — tagging.** `Connection` gains a `window_label: String`, and the
map stays keyed by connection id alone: those ids are `crypto.randomUUID()` per
JS realm, so two windows cannot collide. Only teardown becomes window-aware:

```rust
pub fn close_for_window(&self, label: &str) {
    self.connections.lock().unwrap().retain(|_, c| c.window_label != label);
}
```

**`terminal` — re-keying.** `TerminalManager`'s map is keyed by
**`(window_label, session_id)`**, and `Streaming` carries no `window_label`
field (it would duplicate the key). A terminal session id is the daemon's PTY
handle, deliberately shared between views, so an id-keyed map lets the second
window's `register` evict the first's leg — silently, because the evicted reader
exits on the cancel branch without emitting `transport-dropped`.
`close_for_window` retains on the key's first element.

`close_all()` is *replaced* on both, not kept alongside — leaving it in place is
an invitation to call the app-global version by accident.

For commands, the split follows the same line. `ws_open` gains a
`window: tauri::WebviewWindow` parameter (Tauri injects it) while `ws_send` and
`ws_close` are unchanged, since they look up by a globally unique id. **Every**
terminal command needs the window, because the label is half the key:
`terminal_open`, `terminal_send`, `terminal_resize`, `terminal_resync`,
`terminal_set_theme` and `terminal_close` all take it. No frontend change is
required either way — Tauri fills the parameter in from the calling window.

The transport-half functions (`open_bridge`, `open_terminal_bridge`) take
`window_label: String` rather than a Tauri type, preserving their existing
property of being drivable in tests against a real unix socket with no webview.

### Window close: retire transports, then maybe stop the daemon

`on_window_event` does two things, in this order.

**First, retire the closing window's transports** — `close_for_window(label)` on
both managers. A closing window orphans its connections exactly as a reloading
one does, and `on_page_load` cannot cover it because a window that closes never
loads a page again. This is not optional bookkeeping: with the early return
below, a stranded connection parks its reader task on a socket for the life of
the process, and this app hits macOS's 256-descriptor ceiling for real (see
`fdlimit`). Retiring is idempotent, so the second pass for the same window is a
no-op.

**Then stop the sidecar, but only if this was the last window.**

> **Corrected during implementation — do not restore the earlier version.** This
> spec originally prescribed `if window.app_handle().webview_windows().len() > 1
> { return; }`. That is a bug. `webview_windows()` reads tauri's `AppManager`
> maps, which `AppManager::on_window_close` prunes — and the handler matches
> *both* `CloseRequested` and `Destroyed`. For `CloseRequested` the pruning never
> runs, so the closing window is still counted; for `Destroyed` the `RunEvent`
> callback runs it synchronously *before* per-window listeners, so the window is
> already gone. With windows A and B open, `Destroyed(B)` therefore sees `{A}`,
> `1 > 1` is false, and the daemon dies under A — precisely the bug the guard
> exists to prevent. Verified against the pinned tauri 2.11.2 sources.

The guard filters by label instead, which is immune to the asymmetry:

```rust
let other_window_open = app
    .webview_windows()
    .keys()
    .any(|other_label| other_label != label);
if other_window_open {
    return;
}
```

Everything below that guard is unchanged.

### Targeted transport-dropped

`terminal_open`'s `on_drop` closure captures the window label and uses
`app.emit_to(&label, "terminal:transport-dropped", dropped)`. `open_terminal_bridge`
needs no change — the closure is already its injection seam.

### New window creation

A `open_window(app, route: String)` command builds the window with
`WebviewWindowBuilder`, replicating the `tauri.conf.json` window options
(transparent, `titleBarStyle: Overlay`, hidden title, traffic-light position,
`backgroundThrottling: disabled`) and then calling the extracted
`decorate_window(&window)` — the vibrancy + 60fps block moved verbatim out of
`setup()`.

> **Corrected during implementation — `decorate_window` must be called via
> `window.run_on_main_thread(...)`.** `apply_vibrancy` opens with a hard
> `MainThreadMarker::new().ok_or(Error::NotMainThread)` gate (window-vibrancy
> 0.6.0, `macos/internal.rs:22`), and `open_window` is an `async` command, so
> tauri dispatches it onto the tokio runtime. Called directly it fails *every
> time*, and `decorate_window` only logs the error. That is not "no blur": the
> window is `transparent: true` and `--chrome-bg` is a `color-mix` with
> `transparent`, on the assumption an NSVisualEffectView sits behind it — so the
> second window would show the raw desktop through its own chrome.

**Route delivery.** The target route is delivered by a per-window
initialization script that seeds the hash before any app JS runs, built by the
pure, unit-tested `seed_hash_script(route)`:

```rust
format!(
    "if (!location.hash) {{ location.hash = {}; }}",
    serde_json::to_string(route)?
)
```

The emptiness guard means the route is applied on first load only, so a later
reload preserves wherever the user has since navigated.

> **Corrected during implementation — do not restore the `'#/'` arm.** This
> originally read `if (!location.hash || location.hash === '#/')`. That second
> arm re-seeded the original route whenever a reload found the user back at the
> *root* route, which is the opposite of what the sentence above promises. A
> reload preserves the fragment verbatim, and a hash router at root writes `#/`
> — a non-empty string — so the narrowed guard still fires only on a genuinely
> fresh window. `seed_hash_script`'s tests now forbid the old form.

Rejected alternative: putting the fragment in `WebviewUrl::App("index.html#/…")`.
It probably works (`Url::join` preserves fragments) but it routes a URL fragment
through a `PathBuf`, which is a platform-dependent path with no test coverage in
this repo. The init script is deterministic and needs no frontend change at all.

### Menu — the sole entry point

`New Window` (Cmd+Shift+N) on the existing `Window` submenu (`lib.rs:334`,
currently `.minimize()` only), opening at `/`. That route redirects to
`/ide/<projectId>/home` (`web/src/routes/_shell/index.tsx`) — the project home,
with the full sidebar — so the new window lands somewhere neutral and the user
clicks the workspace they want. Cmd+N is deliberately not used: it is unclaimed
today but reads as "new file", and the in-app new-tab binding is Cmd+T.

**No frontend work ships in v1.** An "Open in New Window" row affordance was
specified and then cut during plan review, for two reasons. First, workspace
rows have no context menu today (`workspace-tree-item.tsx` has no menu
primitive; `PlaceholderRowActions` is a special-case detach button), so it means
introducing a new UI pattern into the sidebar — which existing project notes
flag as sensitive surface. Second, it is pure convenience: Cmd+Shift+N followed
by a click on the target workspace already delivers the entire feature, and the
two windows are identical, which is exactly what was asked for.

Deferred, not forgotten: if the extra click annoys in practice, the follow-up is
an `openWindow(route)` helper in `web/src/lib/crowbar-bridge.ts` wrapping
`invoke('open_window', { route })`, called from a row action. Nothing in this
design blocks it — the command already takes an arbitrary route.

## Accepted limitations (deliberate, documented, not bugs)

- **The same workspace in two windows is permitted and not policed.** Both
  windows debounce-write `crowbar:workspace-layout:<wsId>`, so pane layout is
  last-writer-wins. Policing it needs cross-window awareness of which workspace
  is where — real complexity for a case the user does not have. Revisit if it
  bites.

  This limitation is now **only** about persisted layout. It originally also
  claimed "both windows keep working", which was false until the terminal
  re-keying above: the same workspace in two windows silently killed the older
  window's terminals. That is fixed, so the surviving cost really is just the
  layout write.
- **`sidebar-width`** (`web/src/components/layout/ide-shell.tsx:40`) is one
  shared key. It is read at mount only, so within a session there is no visible
  conflict; across launches the last drag wins. Not worth window identity in the
  frontend.
- **Window size/position are not persisted or restored.** New windows open at
  the configured default.

- **Deep-linking a new window straight to a workspace can bounce to project
  home if the app has only just launched.** `open_window` delivers the route
  correctly — verified with a route the app never navigates to on its own
  (`/workspaces/new`), which lands exactly there — but `WorkspaceRouteGuard`
  evaluates `shouldRedirectUnknownWorkspace` as soon as the workspace list
  resolves, and on a cold start the sidebar may not yet hold that project's
  repos, so the id reads as unknown and the guard redirects. Opening the same
  workspace route once the app is warm works. This does not affect anything
  shipped — the `New Window` menu item passes `/` — but it is the first thing to
  fix if the deferred "Open in New Window" row affordance is built, and the fix
  belongs in the frontend guard, not here.
- **Non-macOS gets the plain builder path.** The decoration block is already
  `#[cfg(target_os = "macos")]`-gated; nothing regresses, nothing is added.

## Non-goals

- Cross-workspace panes within one window.
- Per-window layout persistence for the same workspace.
- Any daemon change whatsoever.

## Verification

Rust unit tests carry the isolation semantics — that is the load-bearing,
regression-prone part, and it is testable without a webview.

**Everything else must be verified live in `make dev-desktop`**, because
multi-window macOS behaviour (vibrancy on a second window, the private-API
60fps un-cap, `titleBarStyle: Overlay` via the builder rather than config) is
exactly what unit tests cannot certify. The decisive manual test:

1. Open window B on a different workspace than window A.
2. Start an agent chat in each; confirm both stream simultaneously.
3. **Reload window B.** Window A's terminals and WebSockets must survive — this
   is the entire point of the transport scoping and the one thing that silently
   regressed before.
4. Close window B. Window A must stay fully live (daemon still up, chats still
   streaming).
5. Confirm window B has blur/vibrancy and correct traffic-light placement.

## Risks

- The WebKit 60fps un-cap and vibrancy on a second window are private-API
  territory and unproven for non-`main` windows. If vibrancy fails on window B,
  it degrades to a non-blurred window — cosmetic, not functional.
- `titleBarStyle: Overlay` plus traffic-light position through the builder
  rather than `tauri.conf.json` is occasionally fussy about ordering.
- This branch has a concurrent agent session writing to it. All work must stay
  inside the files listed in the plan, and commits must name explicit paths —
  never `git add -A`.
