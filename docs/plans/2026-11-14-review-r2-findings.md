# Review R2: ui, sidebar, tab_bar, layout_view, settings_ui, startup_view, pane_host, webview, desktop/app, model

Reviewer R2, read-only. Paths are relative to `app/`. Committed code at HEAD 1706b47db was reviewed. The working tree has uncommitted WIP (`shell_data`, `data_signal`, and an edited `shell_view.rs`). Where that WIP already addresses a finding, the finding says so.

## Method and what was not done

- Read in full: `webview`, `pane_host` (host, slots, protocol, routes, frames), and `desktop/app` (all files at HEAD). Read `model` selectively to spot-check the R1 fixes.
- `ui`, `sidebar`, `tab_bar`, `layout_view`, `settings_ui` and `startup_view` got only targeted reads: overlay plumbing, hover, public surface, and a mechanical manifest scan. Their verdicts rest on that scan and on the rule-script result, not on a full read. Treat them as "not fully reviewed".
- `tools/scripts/check-rust-rules.sh app`: clean for every reviewed crate except `desktop/app`. For `app` it reports 4 `test-missing`, 3 `multi-type`, 2 `type-name`, and 46 `doc-private` (one also in `pane_host/hover_message.rs`). Some of these may come from files edited in the working tree.
- `cargo clippy --workspace --all-targets` and `cargo test` for the reviewed crates were queued behind the shared `rbuild.sh` lock, which other agents held for the whole session. They produced no output, so there is no clippy or test result in this review. Re-run before the gate.
- No online research was done. Rust guidance was applied from the repo's `docs/rust-style.md` and the Rust API Guidelines as already known.

## R1 spot-verification (model)

- M17 (clone per command): fixed. `LayoutHistory::apply` snapshots only for structural commands, and it takes an `Arrangement` instead of the layout.
- M18 (undo memory): fixed. History is a `VecDeque` bounded by `MAX_UNDO_STEPS` and `MAX_PARKED_PANES`, and steps no longer carry pane state.
- M20: `Command::DetachPane` exists.
- M19: not re-verified here.

## Blockers

### B1. The app never stops the daemon gracefully on exit
`desktop/app/src/main.rs`, `daemon_link.rs:27-36`, `daemon_supervisor/src/supervisor.rs:15-20,88-97`
- `Supervisor` documents that the shell must `await stop()`, "the only way the daemon gets SIGTERM and time to flush its state". Nothing in the app calls `stop`.
- `DaemonLink` holds `Option<Arc<Supervisor>>`. `stop(self)` consumes the value, so it cannot be called through the `Arc`.
- On quit, the `Supervisor` drop only `try_send`s a Stop and returns. Then the tokio `Runtime` drops. This cancels the supervision task, and `kill_on_drop` sends SIGKILL to the daemon.
- Failure scenario: the user quits while the daemon is writing, and the daemon is SIGKILLed with no flush. This is a data-integrity risk for a daily-use app.
- Fix: keep the `Supervisor` by value in `DaemonLink` and give `DaemonLink` a `shutdown(self)` that runs `runtime.block_on(supervisor.stop())` with the grace period. Call it after `Application::run` returns and on `cx.on_app_quit`. Add a test with a fake child that asserts SIGTERM was sent.

### B2. Web views never work in a Wayland session
`desktop/app/src/main.rs`, `webview/src/parent_window.rs:21-31`, `gtk_state.rs:49`
- `docs/plans/2026-11-14-gpui-shell-pane-host.md:15` says "On a Wayland session the shell drops `WAYLAND_DISPLAY` so it runs under XWayland". No code does this (grep for `WAYLAND_DISPLAY` and `GDK_BACKEND` finds nothing outside docs).
- GPUI opens a native Wayland window, and `ParentWindow::from_window` returns `UnsupportedParent`. Every pane then shows nothing, and the only trace is a `tracing::error!`.
- The same applies to GTK on its own thread, which would pick the Wayland backend.
- Fix: before `Application::new()`, and before any other thread starts, remove `WAYLAND_DISPLAY` and set `GDK_BACKEND=x11` when `target_os = "linux"`. This is an `unsafe` env call in edition 2024, so put it in one small platform function with a `SAFETY:` comment. Add a test for the decision function. Show an in-window error if the parent cannot be extracted.

## Major

### M1. Origin registration races the first page request
`pane_host/src/pane_host.rs:362-387` (`assign_slot`, `take_slot`), `slot_factory.rs:51-70`
- `factory.create` builds the web view with the entry URL, so the load starts immediately on the GTK thread. The `WebViewId` is only known after `create` returns, and `origins.set(...)` happens after that on the GPUI thread.
- `PaneProtocol::handle` answers `403 unknown web view` when the origin is missing.
- The pooled-slot path is worse: `release_slot` calls `origins.remove`, then `take_slot` calls `navigate(&url)` before `assign_slot` calls `origins.set`.
- Failure scenario: a pane opens as a blank or 403 page when the scheme request wins the race. It is rare, so it looks like a flaky pane in daily use.
- Fix: register the origin before the load can start. Either create the view on `about:blank` (or hidden with no URL), set the origin, then `navigate`; or let `WebViewHost::create_in` take a pre-create hook that receives the id. Set the origin before `navigate` on the reuse path. Add a test with a recording platform that asserts the order of `origins.set` and `navigate`.

### M2. Relay tokens are never revoked and expire under a long-lived slot
`pane_host/src/slot_factory.rs:67-82`, `protocol/src/daemon_transport.rs`, `transport/src/relay_config.rs:47`
- The relay endpoint is requested once per slot at creation (scope pane id `slot-N`). It is baked into the initialization script and is not refreshed.
- The `DaemonTransport` trait has no revoke, so destroying an extension slot (`SlotPool::release` removes it) leaves its token valid until the 12 hour TTL.
- A pooled core slot lives for the whole session. After 12 hours the page's WebSocket reconnect (sleep and wake, daemon restart) is refused with no way to get a new token.
- The scope names the slot, not the pane, so the daemon and the relay cannot tell which pane a stream belongs to.
- Fix: add `revoke_pane` to the transport trait and call it when a slot is destroyed. Either issue the token on assignment, with the pane id as scope, and re-issue it through the IPC channel before expiry, or make the token TTL sliding. Test: slot drop revokes, and a reconnect after TTL gets a fresh endpoint.

### M3. Shipped stubs and demo data in the committed shell
`desktop/app/src/shell_view.rs:326` (`DemoData::reference()`), `sidebar_action.rs`, `settings_modal.rs`, `shell_state.rs:note`, `shell_view.rs:311`
- The sidebar shows `DemoData`. Every sidebar command except opening a thread or Settings returns `SidebarAction::Unserved` and only logs.
- Settings runs on `FakeBackend` ("values live in memory only"), so changes are lost on restart.
- Pane requests, subscriptions, settings and composer calls (`RequestMade`, `Subscribed`, `ShellCall`) are dropped to a debug log, so a page that awaits an answer never gets one. `RunCommand` is also only logged, so the keybinding feature is dead. `PaneHost::set_keybindings` and `extension_status` have no caller.
- `working: false` is hard-coded in `member()`.
- `CLAUDE.md`: "No stubs that ship: a UI must not be wired to a method that returns [] or false".
- The uncommitted `shell_data`/`SettingsAdapter` WIP replaces the sidebar data and settings backend. It does not yet replace pane requests, subscriptions and commands.
- Fix: do not merge until the sidebar data, the settings backend, pane requests and commands are real. Unserved commands must show an error, not a log line.

### M4. A corrupt `layout.json` prevents the app from starting
`desktop/app/src/main.rs:42` (`ShellState::new(...)?`), `layout_store.rs:load`
- `load` returns `Error::Parse`, and `main` returns the error, so the process exits.
- Failure scenario: a power loss or an old format leaves a bad file, and the app never opens until the user finds and deletes it.
- Fix: on `Parse`, rename the file to `layout.json.bad-<ts>`, start with an empty layout, and log it. Test with a garbage file.

### M5. The layout is saved synchronously, with fsync, on the GPUI thread for every state change
`desktop/app/src/shell_state.rs:apply`, `layout_store.rs:save`, `event_commands.rs`
- Every `StateChanged` becomes `SetPaneState`, which is a layout change. That runs `serde_json::to_string` of the whole layout, then `File::create`, `sync_all` and `rename`, on the main thread.
- `apply` also clones the whole layout before and after to detect a change.
- Failure scenario: a page posts state while the user types, and each post stalls the frame by an fsync (milliseconds to tens of milliseconds on a busy disk).
- Fix: write on a background task with coalescing (latest value wins, written at most every N ms and on quit). `LayoutHistory::apply` should report whether anything changed, which removes the clone.

### M6. Quitting or closing during a pending hover/leave order bug clears the wrong pane's hover
`desktop/app/src/shell_view.rs:305-309` and `layout_view/src/layout_view.rs:79-83`
- `apply_outcome` turns `(pane, false)` into `set_hovered_pane(None)`, and `set_hovered_pane` overwrites unconditionally.
- Pointer moves from web view A to web view B. If B's `true` is handled before A's `false` (two web views post independently through the shared queue), A's `false` clears B's hover. The tab shows no hover fade and the pane keeps the wrong chrome until the pointer moves again.
- Fix: clear only when the current hovered pane equals the pane that reported `false`. Add a test with the order B-true then A-false.

### M7. `PaneHost` keeps a second copy of pane state and the doc says otherwise
`pane_host/src/pane_host.rs:30-33` (doc "keeps no copy"), `pane_host_frames.rs:remember_state`
- `remember_state` writes the new state into `runtime.instance.state`. `sync` also overwrites `runtime.instance` from the snapshot (a full clone of every pane, up to 64 KB each, on every sync, including each window resize frame).
- If the model refuses `SetPaneState` (size cap), the host copy diverges from the model until the next sync.
- `CLAUDE.md`: never a second copy of state. Fix: do not store state in the runtime. Read state from the snapshot when building a lifecycle frame, or hold only what is needed to send the frame. Do not clone `PaneInstance` on every sync; compare ids and params instead.

### M8. Errors abort the whole reconcile and can leak a slot
`pane_host/src/pane_host.rs:sync, retire_removed, release_slot:322-336`
- Every step uses `?`. One pane that fails `send` or `hide` ends `sync`, so the remaining panes are neither placed nor retired.
- In `release_slot`, `runtime.detach()` runs first, then `hide()?` and `navigate()?`. If one fails, `pool.release` and `origins.remove` never run: the slot stays owned forever and the origin entry stays.
- Fix: finish the release (clear owner, remove origin) unconditionally and collect errors; make `sync` best-effort per pane and return the first error at the end.

### M9. Slot reuse can attribute a stale message to the new pane
`pane_host/src/slot_factory.rs:deliver`, `pane_host.rs:release_slot/take_slot`
- The owner is read when the GTK thread runs the IPC handler, but `release_slot` clears the owner and `take_slot` reassigns the slot to another pane in the same call, while the old page is still alive (`navigate("about:blank")` is asynchronous).
- A message the old page posts after the reassign is delivered as the new pane's message. A stale `Register` with the right type id marks the new pane mounted too early, and a stale `SetState` overwrites its state.
- Fix: stamp each assignment with a generation counter and drop IPC from an older generation. Only reuse a slot after the blank navigation is confirmed, or destroy the view.

### M10. Production pane bundle path defaults to the current directory
`desktop/app/src/app_options.rs`: `panes_dir` defaults to the relative path `panes`.
- A launcher that starts the app from `/` or a desktop entry finds no bundle, and every chat is a 404 page.
- Fix: resolve relative to the executable (or an embedded bundle) and fail with a startup error that names the missing path.

### M11. A daemon outage after the hand-over is invisible
`desktop/app/src/shell_view.rs:feed_startup, on_startup`
- After `HandedOver` the startup view is dropped, so `feed_startup` has nowhere to send `Unavailable` or `Recovering`.
- A daemon crash during use shows nothing, and panes just fail their requests. The Tauri app surfaced this. Fix: recreate the startup overlay (or a banner) on any non-Ready phase after hand-over.

## Minor

- m1. `webview/src/linux_platform.rs:create` blocks the GPUI thread on `answer.recv()` until the GTK thread builds the view. A slow WebKit init freezes the whole window.
- m2. `webview/src/linux_platform.rs`: the GTK thread is never joined and `main_quit` is not requested on drop. On exit the web views are torn down by process teardown.
- m3. `webview/src/macos_platform.rs`: `with_view` holds the `RefCell` borrow while running view calls. An IPC callback that ends in `destroy` (`borrow_mut`) would panic. Not compiled or run on Linux, so this is unverified.
- m4. `pane_host/src/pane_host.rs:handle_ipc`: every message is JSON-parsed twice (`parse_hover`, then `decode_pane_frame`), and the inbox is unbounded with no message size cap. A runaway page can grow memory without limit.
- m5. `pane_host/src/policy.rs`: an extension page may call `/v0/ext/<own>/` on any host id. Check this is intended. `starts_with` on a path that may contain encoded dot segments relies on the engine to normalise.
- m6. `pane_host/src/dir_bundle.rs`: the traversal check uses `Path::components`, which is fine, but the path comes from the URL without percent-decoding. A bundle file with a space or non-ASCII name cannot be served.
- m7. `webview/src/gtk_state.rs:builder_for`: a related view is looked up with a linear scan of all views for every core view created. Fine now, but it chooses an arbitrary related view.
- m8. `desktop/app/src/daemon_link.rs`: `MemoryTokenStore` in production means remote host credentials are not persisted. This is acceptable only while no remote host is wired.
- m9. `desktop/vendor/gpui-x11-window-handle.patch`: `borrow_raw` has no `SAFETY:` comment, and `screen_id` is taken from the display id, not the XCB screen number. wry does not use it today.
- m10. HiDPI: `to_rect` sends logical coordinates to a child X11 window. If GTK and GPUI disagree on the scale factor, panes are misplaced. Unverified; needs a test at 2x.
- m11. Windows of the `Workspace` canvas fire `Measured` on every resize frame. Each one runs a full `PaneHost::sync` (see M7 for the clone cost).

## Nit

- n1. `desktop/app/src/shell_view.rs` (`#[expect(clippy::trivially_copy_pass_by_ref)]` twice) is fine, but `on_tabs` ignores events with `_ => {}`; make the match exhaustive so new events are not silently dropped.
- n2. `startup_command.rs`: `_ => {}` swallows future `StartupCommand` variants.
- n3. `pane_host/src/bootstrap.js`: `chord` is computed per binding in the `find` closure. Compute it once per event.
- n4. The `gallery`, `sidebar_gallery`, `tab_bar_demo`, `layout_demo` and `settings_preview` binaries ship in the crates. Confirm they are dev-only (`required-features` or `examples/`).

## Public surface (Rabbyte rule 14, API Guidelines C-STABLE)

| crate | `pub use` at root |
|---|---|
| ui | 62 |
| sidebar | 75 (includes `DemoData`, `drag_started`, `edge_speed`, `allowed_modes`) |
| settings_ui | 53 (includes `FakeBackend`) |
| pane_host | 21 |
| webview | 16 |
| layout_view | 13 |
| startup_view | 13 |
| tab_bar | 10 |
| model | 18 |

`sidebar` and `settings_ui` export test and demo support (`DemoData`, `FakeBackend`) from the production API. `CLAUDE.md` says no test-only items in non-test files; move them behind a `test-support` feature. With 60 to 75 items each, `ui` and `sidebar` cannot be reviewed as a coherent API. A `deadcode` pass (pub items without callers) was not run.

## Rabbyte manifest, rule by rule (mechanical scan, counts per crate)

Scope: non-test source files, up to the `#[cfg(test)]` marker. Counts are from a regex scan, so they are approximate (over-counts on R1 and R2, under-counts on R5). Rule 42/43 (sibling test files) are not counted, per the owner change. Rules 9 to 28 are Go-specific and replaced by Rust equivalents (R9 `Error` enum with `thiserror`: followed in every crate; R24 no `static mut`/globals: followed, `unsafe_code` is denied; R25, R27 and R26: see the check-script result).

| rule | ui | sidebar | tab_bar | layout_view | settings_ui | startup_view | pane_host | webview | app | model |
|---|---|---|---|---|---|---|---|---|---|---|
| files / non-test LOC | 127 / 8078 | 129 / 7191 | 28 / 1856 | 29 / 1839 | 113 / 6650 | 22 / 1626 | 38 / 1843 | 24 / 1403 | 26 / 1698 | 27 / 2029 |
| R1 several parameters on one line | 256 | 186 | 57 | 79 | 219 | 38 | 61 | 59 | 47 | 64 |
| R2 function nesting over 2 | 9 | 19 | 5 | 5 | 14 | 8 | 7 | 4 | 15 | 8 |
| R3 `else` | 17 | 13 | 6 | 0 | 16 | 4 | 4 | 1 | 2 | 7 |
| R5/R32 magic number literal | 146 | 97 | 10 | 7 | 74 | 23 | 10 | 3 | 1 | 0 |
| R33 bool flag parameter | 5 | 2 | 0 | 0 | 1 | 0 | 0 | 0 | 1 | 0 |
| R7 inline comment (non-doc) | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 |
| R7 doc comment on private item | 0 | 0 | 0 | 0 | 0 | 0 | 1 | 0 | 46 | 0 |
| R8 more than one top-level type | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 3 files (`request_sinks`, `shell_view`, `workspace`) | 0 |
| R44 non-test part over 500 lines | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 |
| `unwrap`/`expect`/`panic` outside tests | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 |

How to read these:
- R1 as written ("one parameter per line, always") conflicts with `rustfmt`, which wraps only when a line is too long. The 1 000-odd hits are formatter-conformant code. Either state the rule as "rustfmt output" for Rust or accept the count as N/A. I did not list them as defects.
- R2: the scan counts `if`, `match`, `for`, `while`, `loop` and closure blocks inside a function. Hits in `ui` (render builders), `sidebar` and `settings_ui` are mostly closures within `.when(...)` chains. Real offenders to start with: the listed `sidebar` (19) and `app` (15) functions.
- R3: most hits are `if ... else` used as an expression (for example `ui/src/checkbox.rs:78,92`, `button.rs:132`). `let ... else` is not counted. Statement `else` after a returning branch is rare; sample the list in `sidebar` and `ui/src/tab_cell.rs:90-112`.
- R32: the magic-number count is high in `ui` (146), `sidebar` (97) and `settings_ui` (74), driven by pixel sizes, opacities and durations written inline (`px(20.0)`, `.opacity(0.32)` in `ui/src/checkbox.rs`). A design token layer (`ui` theme constants) exists; these literals bypass it.
- R33: bool parameters at `ui/src/checkbox.rs:64`, `pose.rs:14`, `slide_state.rs:37`, `switch.rs:69`, `tween.rs:28`; `sidebar/src/removal_tray.rs:85`, `terminal_threads.rs:14`; `settings_ui/src/value.rs:64`; `app/src/titles.rs:24`. The `on_change(bool, ..)` callbacks are the value, not a flag, so only some are violations.
- R7 (doc on private items): 46 in `desktop/app/src` plus `pane_host/src/hover_message.rs:1`. The check script reports them as `doc-private`; remove them or make the items non-private.
- R8: `request_sinks.rs` has `SidebarRequests`, `StartupRequests` and `OverlayRequests`; `shell_view.rs` has `ShellParts` and `ShellView`; `workspace.rs` has `WorkspaceEvent` and `Workspace`. The script also flags `pane_source.rs` and `shell_outcome.rs` as type-name mismatches (`ShellPaneSource`, `Outcome`) and `host_builder.rs`, `settings_modal.rs`, `startup_command.rs`, `workspace.rs` as having no test. The files `daemon_link.rs`, `shell_state.rs` and `workspace.rs` contain doc comments on `pub(crate)` items.
- R38 (constructor injection): `pane_host`, `webview` (`HostConfig`, `Platform`), `DaemonLink` and `ShellView` take their dependencies as arguments. The exception is `ui::Overlays` and `ui::Theme`, which are GPUI globals (`cx.global`). This is the framework's own idiom, but it is hidden state, so note it.

## Tests

- I read test names and layout only; the suite was not run (see Method).
- `pane_host` has recording platform tests for sync and frames, which is good. Missing: the M1 ordering, the M9 stale-message case, a failure during `release_slot` (M8), and the relay token lifecycle (M2).
- `desktop/app` has no test for exit and shutdown (B1), corrupt layout (M4), or hover ordering (M6).
- `daemon_supervisor` tests (outside this scope) use real `sleep` in `stop_tests.rs:18,27` (`exec sleep 30`, a child process, not a test-thread sleep) and `tokio::time::sleep` under paused time in `watchdog_tests.rs:78,81`; these look legitimate.
- `settings_ui/src/intent_tests.rs` contains no `assert`. Check that it tests something, or delete it.

## Verdict per crate

| crate | verdict | reason |
|---|---|---|
| webview | fix first | B2 (Wayland), m1 (blocking create), M1 depends on its API |
| pane_host | fix first | M1, M2, M7, M8, M9 |
| desktop/app | fix first | B1, B2, M3, M4, M5, M6, M10, M11, rule-script failures |
| model | ship | R1 fixes verified (M17, M18, M20); no new defects found in the parts read |
| layout_view | fix first | M6 (the `set_hovered_pane` contract); otherwise only partly reviewed |
| ui | ship, not fully reviewed | overlay guard and host are sound; public surface (62) and R32 literals to trim |
| sidebar | fix first | exports demo data in its API; R2 count of 19; not fully reviewed |
| settings_ui | fix first | exports `FakeBackend`; not fully reviewed |
| tab_bar | ship, not fully reviewed | only the scan was applied |
| startup_view | ship, not fully reviewed | only the scan was applied; M11 is in the app |

## Counts

Blocker 2, Major 11, Minor 11, Nit 4. Verdicts: ship 4 (model, ui, tab_bar, startup_view; the last three not fully reviewed), fix first 6.
