# Production-readiness review: GPUI shell (migration/rust-shell)

Date: 2026-11-16. Reviewer: independent, read-only (no builds, no tests run).
Scope: `app/crates/{protocol,model,transport,browser_nav}` and `app/desktop/{app,crates/*}`; `vendor/` excluded.
Sampled in depth: `pane_host`, `webview`, `daemon_supervisor`, `transport` (relay, gate, pinning, keychain, local, pairing), `model` (layout, history), `shell_data` (runner, hub, host_stream, icon cache), `terminal_session`, `terminal_view`, `app` (`shell_state`, `daemon_link`, layout store/writer). Spot-checked: `shell_platform`, `browser_nav`, `protocol` (state cap, URL/path newtypes), `app/panes` SDK as the other side of the IPC contract.
Not reviewed in depth: `sidebar`, `settings_ui`, `command_palette`, `ui`, `tab_bar`, `layout_view`, `startup_view`, `pairing_view`, `remote_folder_picker`, `branch_import_view`, `terminal` emulator internals, `window_state`, `shell_keys`. Findings there are possible and not excluded.

## Overall verdict

Not production-ready until the blocker and the majors marked "gate" are fixed. The codebase is unusually disciplined: no `unwrap`/`expect`/`panic!` on any non-test runtime path (only in two demo binaries), pinned TLS with signature verification delegated to rustls, a token-gated loopback relay with Host/Origin/path checks and connection caps, a bounded and validated layout model, bounded undo memory, and a strict 64 KiB `PaneState` that is enforced on deserialize as well as on construction. Most of the risk is concentrated in (a) one trust-boundary hole (daemon-controlled strings used as file names), (b) shell features that the SDK advertises but the shell silently does not serve, (c) a handful of lifecycle/backpressure gaps, and (d) macOS parity.

Severity: BLOCKER = do not ship; MAJOR = fix before release or consciously accept with an issue; MINOR = fix when touched.

---

## BLOCKER

### B1. Arbitrary file write on the desktop from daemon-supplied project id / icon version (path traversal)
- `app/desktop/crates/shell_data/src/icon_cache.rs:32-39` (`path_of`), `icon_fetch.rs:82-90` (`save`).
- `path_of` builds `dir.join(format!("{project}-{version}"))` where `project` is the daemon's project id and `version` is everything after `?v=` in the daemon's `avatarUrl`. Neither is validated. `RequestTarget` (`protocol/src/request_target.rs:11-27`) only rejects spaces, controls, `\`, `#` and non-ASCII, so `..` and `/` pass. `save` then runs `create_dir_all(parent)` and `std::fs::write(path, bytes)` with the response body, which the same host chooses.
- Failure scenario: a paired remote host (or a compromised local daemon) sends a project entity with `"avatarUrl":"/v0/projects/p/icon?v=../../../../.ssh/authorized_keys"` (or a project id containing `../`). The shell fetches the URL from that host and writes the attacker's bytes to `<icons>/p-../../../../.ssh/authorized_keys`, i.e. any user-writable file (`~/.bashrc`, `~/.ssh/authorized_keys`, `~/.config/autostart/*`). This is code execution as the user from a host that is only supposed to be semi-trusted.
- Fix: never derive a path from remote text. Name the file from a hash of `(host id, project id, url)` or accept only `[A-Za-z0-9_-]{1,64}` for both pieces, canonicalize the result and assert it is under `dir`, write via temp + rename, cap the size (see M12), and do the write on a blocking thread. Add a test with `v=../x` and `id=../x`. Also key the cache by host: today two hosts with the same project id share one file name and one `held` slot.

---

## MAJOR

### M1. Pane SDK requests are accepted and never answered; several frames are silent no-ops (gate)
- `app/desktop/app/src/shell_state.rs:299-306` and `:356-376` (`serve_frame`); frames routed in `pane_host_frames.rs:96-117`.
- `HostEvent::RequestMade`, `Subscribed`, `Unsubscribed`, and `ShellCall` for `SettingsGet`, `SettingsSubscribe`, `ComposerAttach`, `ComposerDraft`, `AttachmentOpen` fall into `other => tracing::debug!("... does not serve yet")`. Only `OpenExternal` and `Reveal` are served.
- The pane SDK (`app/panes/src/sdk/pane-client.ts:88-130`) exposes `request`, `getSetting`, `subscribe`, `subscribeSetting` and wraps them in a `Promise` with no timeout (`call()`), so every one of these hangs forever. Violates "no stubs that ship: a UI must not be wired to a method that returns [] or false".
- Failure scenario: any pane (core chat composer attach/draft, extension pane calling `client.request`) awaits a promise that never settles; UI spinner forever, no error.
- Fix: either serve them (respond with `ShellFrame` result/error ids) or reply with an explicit "unsupported" error frame so the promise rejects, and add an SDK-side timeout. Remove the SDK surface that the shell will not serve in this release.

### M2. Extension panes are unreachable: bundles, status and pane types are never wired (gate)
- `app/desktop/app/src/host_builder.rs:24` uses `ExtensionBundles::default()` and nothing ever calls `ExtensionBundles::insert`, `PaneHost::extension_status` (`pane_host.rs:296`), or registers an extension `PaneType` (only `register_core_panes` registers anything).
- All extension-scoped machinery (`policy.rs`, `access_scope.rs`, per-extension web contexts/data stores, `x-crowbar-extension` header, `apply_status`/`recover`) is therefore dead in the shipped binary, and untested end to end. Per project memory, "backend extensions fully working" is a merge gate.
- Fix: wire daemon extension status/list into `ExtensionBundles` + `PaneRegistry` + `extension_status`, or delete the unreachable surface and track it in an issue (deadcode gate).

### M3. Pane IPC inbox drops frames silently, including `register`
- `pane_host/src/slot_factory.rs:17,137-139`: bounded channel of 256, `try_send` failure only logs. A pane that bursts `set_state`/`set_title` (editor keystrokes, console output) while the UI thread is busy can lose its `register` frame, after which the pane never mounts and stays in `PanePhase::Loading` with no error and no retry; dropped `request` frames never get answers either.
- Failure scenario: app startup with several panes loading while the main thread is busy (layout sync, snapshot, large settings push) and one noisy pane.
- Fix: coalesce by (pane, kind) for state/title (latest wins) and never drop control frames: treat a full inbox as backpressure for `register`/`request`, or fail the pane visibly. Count and surface drops.

### M4. Terminal PTY is killed on any transient stream interruption; no reattach
- `terminal_session/src/terminal_sessions.rs:111-120` and `binding.rs:30-40`: when `pump` returns for any reason (daemon restart, relay idle close, WS error), `attach` calls `binding.release`, which `DELETE`s the session for `Binding::Create`. `stream.rs:18-24` ends on the first side to finish; there is no reconnect and the view only gets a closed channel.
- Failure scenario: the supervisor restarts a wedged daemon, or the laptop sleeps; every terminal's shell and its child processes are killed and the pane is a dead grid. Also no connect/handshake timeout: `TcpStream::connect` and `client_async` at `:136-143` can hang a terminal at "Starting" forever.
- Fix: distinguish "view dropped" (kill) from "stream lost" (keep PTY, reattach with the existing session id and the daemon's snapshot frame, show a "disconnected, reconnecting" line); add timeouts.

### M5. Daemon stdout/stderr are piped but never drained when the log cannot be opened
- `daemon_supervisor/src/spawn_daemon.rs:37-38` always uses `Stdio::piped()`, but `:47` only attaches the pump when `supervision.log` is `Some`. `Supervision::new` sets `log = None` on any open failure (`supervision.rs:20-26`, e.g. read-only or full home, bad permissions).
- Failure scenario: the daemon writes to stderr, the 64 KiB pipe fills, and the Go daemon blocks on write: wedged daemon, watchdog restarts it, loop until the restart budget is exhausted, "gave up".
- Fix: use `Stdio::null()` (or inherit) when there is no log sink; surface the log-open error as a notice.

### M6. Startup can hang before any window: `crowbar-api paths` has no timeout and runs inside `block_on` on the main thread
- `app/src/daemon_link.rs:56-57` (`runtime.block_on(try_launch ...)`), `:212` and `daemon_supervisor/src/daemon_paths.rs:64` (`command.output().await`, no timeout, no `kill_on_drop`).
- Failure scenario: a stale or hung `crowbar-api` found on `PATH` (`locate_binary` also searches `PATH`, `locate_binary.rs:111-114`, and does not check the exec bit) makes the app show nothing and never reach the startup/halt view that exists for exactly this case.
- Fix: wrap in `tokio::time::timeout`, set `kill_on_drop(true)`, and do launch asynchronously after the window exists so the startup view can show the failure. Prefer the binary beside the executable and do not fall back to `PATH` in release builds.

### M7. Local daemon socket is trusted without ownership or peer-credential checks
- `transport/src/local.rs:39-45` connects to whatever is at the path; no `SO_PEERCRED`/`LOCAL_PEERCRED` or owner/mode check anywhere in the workspace. `spawn_daemon.rs:13-21` creates the socket directory with `recursive(true)` + mode 0700, which is a no-op on an existing directory, so a pre-created directory keeps its mode and owner. `pid_mismatch` (`daemon_supervisor.rs:114-124`) only logs "a stale daemon may own the socket".
- Failure scenario: if the path from `crowbar-api paths` is under a shared location (the unit test uses `/tmp/crowbar-1f.sock`), another local user pre-creates the directory/socket and receives every pane request, including project/file contents and terminal I/O.
- Fix: after `connect`, check peer uid equals the current uid; verify the socket dir is owned by the user with mode 0700 and refuse otherwise; treat `pid_mismatch` as an error (restart/clean the socket), not a warning.

### M8. Relay token is fixed for the page and expires after 12 hours; panes cannot refresh it
- `transport/src/relay_config.rs:8,55` (`token_ttl` 12 h), `token_book.rs:43-55` (checked at every connect), `pane_host/src/bootstrap.js:6-13` (fetched once, synchronously, at page load), `app/panes/src/lib/ws/shell-socket.ts:8` (reads the cached `ws`).
- Failure scenario: a chat/terminal-adjacent pane left open overnight; any WebSocket reconnect after 12 h gets 401 and the pane stays disconnected until reloaded. Conversely a stolen token is valid for 12 h and reusable.
- Fix: serve a fresh endpoint per connect (`/__crowbar/relay` is already an on-demand route; expose it as an async call in the SDK) with a short TTL (minutes) and single use; keep `revoke_pane`.

### M9. UI thread blocks on web-view operations
- `webview/src/linux_platform.rs:137-141` (`create` waits on `answer.recv()` with no timeout) and `:193-204` (`snapshot` waits up to 400 ms). `pane_host_freeze.rs:21-45` snapshots every visible pane sequentially on the GPUI thread each time an overlay opens (palette, settings, dialogs).
- Failure scenario: 6 visible panes with a slow WebKit process = up to 2.4 s freeze whenever an overlay opens; a hung GTK thread freezes the entire app (`create` blocks forever). On timeout the pane silently shows nothing (`Ok(None)`).
- Fix: make `create` asynchronous (reply channel polled by the GPUI executor) or bound it; take snapshots concurrently (post all jobs, then collect with one shared deadline) or capture lazily/in the background and keep the last frame.

### M10. macOS parity gaps in the web view layer
- `webview/src/platform.rs:88-93` default `snapshot` returns `Ok(None)`, and `macos_platform.rs` does not override it: on macOS every overlay hides the web panes and paints nothing in their place (blank holes behind the palette/settings).
- `macos_platform.rs:62-75` uses `with_data_store_identifier`, which needs macOS 14+; no deployment-target guard or fallback is visible. `HostConfig::data_dir` is ignored on macOS. `shell_platform` Linux/mac behaviours and `window_vibrancy.rs` were not exercised here.
- Fix: implement `WKWebView.takeSnapshot` for macOS, document/enforce the minimum macOS version, and add the parity item to the release checklist.

### M11. Terminal output has no backpressure and the UI applies unbounded batches
- `terminal_view/src/terminal_link.rs:19-20` uses unbounded channels; `terminal_view.rs:272-284` drains everything available into one `Vec` and applies it in a single `view.update` on the UI thread.
- Failure scenario: `cat` of a large file or `yes` produces output faster than the grid processes it: memory grows without limit, and each `update` can run for a long time, freezing the whole window including other panes. The relay accepts 64 MiB messages (see M12) so one frame can be very large.
- Fix: bounded channel (let the WebSocket read stall, which backpressures the PTY), cap per-update work to a time or byte budget and yield.

### M12. Unbounded memory per request/connection in the transport
- `transport/src/local.rs:20` `LOCAL_BODY_LIMIT` = 1 GiB buffered into a `Vec` per response, up to 32 in flight per host (`shell_transport.rs`); `relay_config.rs:9` `DEFAULT_MAX_MESSAGE_BYTES` = 64 MiB x 256 connections; `PaneProtocol` applies no limit to request bodies from a page (`pane_protocol.rs:151-163`). `icon_fetch::save` buffers whatever a host sends.
- Failure scenario: a misbehaving pane/extension/host can drive tens of GB of resident memory.
- Fix: stream large bodies or cap at a sane size (tens of MB) with a typed error; reduce the per-message relay cap to what a terminal snapshot needs and apply a global budget.

### M13. Layout and settings persistence failures are swallowed
- `app/src/layout_writer.rs:66-73` logs `saving the layout failed` and moves on; `ShellState::apply` calls `writer.submit` and nothing reports failure. Same pattern: `shell_data/src/setting_file.rs`, `host_notes.rs:115-117` use non-atomic `std::fs::write`.
- Failure scenario: disk full or permission change: the user keeps working, quits, and loses the layout silently; a crash during `host_notes` write truncates the file.
- Fix: surface a persistent notice after N consecutive failures, retry on next change, write atomically (temp + fsync + rename + dir fsync, mode 0600) everywhere; `layout_store.rs:80` creates files with default umask.

### M14. A malformed/oversized pane frame permanently fails the pane
- `pane_host.rs:202-205` + `reject` sets `PanePhase::Failed` for any undecodable frame; recovery exists only for extension status. A single bad message from a page (including a legitimate pane that exceeds the 64 KiB state cap at `PaneFrame::SetState`) turns the pane into an error card until the user closes it. Also `deliver` drops >1 MiB messages without telling the pane (`slot_factory.rs:123-126`).
- Fix: reply with a protocol error frame and keep the pane alive for recoverable errors (too large state, unknown kind); fail only on repeated violations.

---

## MINOR

### Security and trust boundary
- m1. `pane_host/src/pane_protocol.rs:92-104`: the relay lookup spawns one OS thread per request with no cap, and if `thread::Builder::spawn` fails the responder is dropped without a reply. `bootstrap.js:7-9` performs that lookup with a synchronous XHR, so the page's main thread would hang forever. Reply with an error on spawn failure, use the runtime instead of a thread, and add a timeout.
- m2. `pane_host/src/policy.rs:16-18`: extension panes get 403 for `/fonts/...` (core bundle only), so extension UIs cannot load the shared UI fonts. Allow `fonts/` for extension origins.
- m3. `app/src/daemon_link.rs:273`: `ExactOrigins::new(vec![CORE_ORIGIN], CORE_ORIGIN)` passes a template with no `{extension}` placeholder, so extension relays accept exactly the core origin. Isolation then rests only on the token scope and `ensure_path_allowed` (which is sound), but the comment/contract of `ExactOrigins` is violated. Use a real per-extension origin or drop the parameter.
- m4. `transport/src/cached_token_store.rs:9-19`: the cache keeps the full credential JSON (bearer token) as plain `String` for the process lifetime, defeating the `Zeroizing` in `Credential`. Cache a `Credential`/`Zeroizing<String>`. Also on Linux `PlatformFailure` is treated as non-denial and never cached (`os_keychain.rs:21-32`), so a headless box without a Secret Service retries (and may spawn D-Bus calls) on every request.
- m5. `shell_platform::follow_link` (`host_builder.rs:37`) hands any `http(s)/mailto/tel` navigation of a pane to the OS with no user-gesture check, from any frame; a pane/extension can spam the default browser or mail client. Rate-limit or require a recent user input. `PaneFrame::Reveal` (`shell_state.rs:361`) lets any pane reveal any absolute path; restrict extensions to paths under their workspace.
- m6. `pane_host/src/pane_host_slots.rs:29-43`: when an idle slot is reused for another pane, the old page's late requests are authorized against the new pane's origin entry (the origin table is keyed by web view id, not by page generation). Core-to-core only today, but include the generation in the protocol authorization.
- m7. `browser_slot.rs:41`: the navigation filter rejects `about:srcdoc`, `blob:` and `data:` for subframes too (`BrowserUrl::is_loadable`, tested as refused), which breaks many real sites' iframes. Wry's navigation handler does not say which frame navigates, so allow `about:srcdoc` and `blob:` of an already-allowed origin or drop the subframe case from the filter.
- m8. `daemon_supervisor/src/locate_binary.rs:91-93`: `is_executable_file` checks `is_file` only; an `explicit` path is returned unchecked.
- m9. `transport/src/pin.rs`: pairing compares a 96-bit fingerprint (matches the daemon format) and later dials pin the full SHA-256; acceptable, but the user-typed fingerprint is compared as text, so confirm it is normalized (case, spacing) before comparison.

### Correctness and races
- m10. `shell_data/src/host_stream.rs:54-56`: the reconnect `sleep(backoff.wait())` is not interruptible, so a host removal (`StreamControl` channel closing) or `Resubscribe` waits up to 30 s; controls queue on an unbounded channel meanwhile. Select on `controls.recv()` during the sleep. Add jitter (all hosts retry in lockstep).
- m11. `host_stream.rs:134-143`: `Frame::Closed` for any scope other than `projects`/`resync` is only logged, leaving the link "Live" with stale data. Mark the host degraded and resubscribe or surface the reason.
- m12. `shell_data/src/runner.rs:29-38`: `committing` is set before and cleared after `carry_out`; if the task is cancelled or panics it stays true forever. Use a guard. Multi-call plans have no rollback: a failure after the first call marks the row failed although earlier calls succeeded, so a retry duplicates.
- m13. `app/src/shell_state.rs:339-348`: every `Loaded` event of a browser pane applies a `SetPaneState` and clones/serializes the whole layout (`apply` -> `writer.submit(layout.clone())`). Same for every `set_state` keystroke from an editor: O(total pane state) per event (up to 64 KiB x pane count). Coalesce state writes (debounce in the writer, which already coalesces) and avoid the clone by sharing an `Arc`.
- m14. `shell_state.rs:332-336`: a page title that fails `PaneTitle::new` is dropped silently (over-long or control characters), leaving the stale title; truncate instead.
- m15. `terminal_session/src/stream.rs:30-33`: `message.ok()` discards WebSocket errors in `filter_map`; the user sees no reason for a dead terminal. Emit a notice frame (like `run` does for connect errors).
- m16. `shell_data/src/icon_cache.rs`: old icon versions are never deleted (disk growth, one file per avatar version); `icon_fetch.rs:82-90` does blocking `std::fs` I/O on a tokio worker.
- m17. `app/src/shell_view/data_events.rs:17` and `shell_data/src/plan_icon.rs:34` read user-picked files with `std::fs::read` on the UI/async thread with no size cap (descriptor install, space icon). Use `spawn_blocking` and a cap.
- m18. `daemon_link.rs:134-141`: `Arc::try_unwrap(supervisor)` fails while a restart task holds a clone, so shutdown falls back to `kill_on_drop` (SIGKILL) and the Go daemon does not shut down cleanly; also terminal sessions are not killed at app exit (`TerminalSessions` only kills when the stream ends), so PTYs can outlive the app until the daemon cleans them.
- m19. `pane_host/src/dir_bundle.rs:44-50`: reads whole files with `std::fs::read` on the protocol path (GTK thread/handler), follows symlinks inside the bundle directory, and `route.rs:43-44` leaves a `?query` in `fonts/` paths so `/fonts/x.woff2?v=1` is a 404.
- m20. `transport/src/shell_transport.rs`: the per-host in-flight cap waits up to the 300 s proxy timeout for a slot (`:68`), so 32 stuck requests make later requests queue for five minutes; use a short queue timeout with a 503.

### Hygiene and spec
- m21. The inventory marks the build readout and Developer tab as CUT, but `app/src/shell_view/readout*.rs`, `readout_*.rs` and a 1 s sampling task ship behind the `buildReadout` setting (default off). Remove or record the decision.
- m22. `PaneProtocol` and `policy.rs` duplicate the traversal/extension checks in `transport/access_scope.rs`; defense in depth is fine, but the two copies already differ (marker list, query handling). Keep one source of truth.
- m23. The `webview` crate's `Platform` trait offers `snapshot` with a silent `Ok(None)` default; per the "no stubs" rule, make unsupported platforms return a typed `Unsupported` so callers can show a placeholder explicitly.
- m24. Windows is out of scope per project decision; `webview` and `x11_session` correctly cfg-gate Linux/macOS. No `cfg(windows)` paths found.

---

## What was checked and found sound (so it is not re-reviewed)

- No runtime `unwrap`/`expect`/`panic!`/`todo!` outside tests and two `*_demo` binaries (scripted scan of all non-vendor sources up to each file's `#[cfg(test)]`).
- TLS: custom verifier pins the certificate hash and still verifies handshake signatures (`pinned_verifier.rs`); the unpinned probe verifier is used only for probing (`unpinned_probe_verifier.rs`); a rejected certificate maps to a distinct `IdentityChanged` error (`dial.rs`).
- Relay gate: token in a subprotocol (not the URL), exact `Host`, single `Origin`, token scope checked against origin policy and path, handshake slots separate from stream slots, 503 when busy, revocation on pane release and host removal (`gate.rs`, `relay.rs`, `token_book.rs`).
- Credential handling: token+pin stored as one keychain entry, `Zeroizing` in `Credential`, redacted `Debug`, pairing rolls back registry/keychain/remote token on any failure.
- Custom protocol: authorization is by web-view id assigned by the host (the page never names its identity); extension routes confined to `/v0/ext/<id>/`; `DirBundle` rejects non-`Normal` path components; browser panes get no scheme, no bootstrap script and no IPC handler, and non-http(s) navigations are cancelled.
- `PaneState` 64 KiB cap enforced in `new` and in `Deserialize`; IPC message cap 1 MiB; generation tagging rejects stale page messages (`SlotOwner::resolve`).
- Model: `Layout` deserialization validates (`from_stored` -> `validate`), undo history bounded (64 steps, 256 parked panes), `deny_unknown_fields`, versioned schema.
- Supervisor: bounded broadcast bus, restart budget with exponential backoff, watchdog with goroutine dump then SIGQUIT then kill, log 0600 in a 0700 dir with rotation and a bounded writer queue.
- Layout store: quarantines an unreadable layout instead of crashing; writer thread coalesces and flushes on drop.

## Suggested order of work

1. B1 (icon path), M5 (stdout pipe), M6 (startup hang), M7 (socket trust).
2. M1/M2 (unserved frames, unwired extensions) together with the SDK: decide serve-or-remove before release.
3. M3, M14 (IPC loss and fail-closed behaviour), M4 (terminal reattach), M8 (relay token refresh).
4. M9-M12 (blocking, backpressure, memory caps), M10 (macOS snapshot) with the parity checklist.
5. Minor items as the files are touched; add a failing test first for each (project rule).
