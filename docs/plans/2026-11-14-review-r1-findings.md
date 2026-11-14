# Review R1: protocol, model, transport, daemon_supervisor

Scope: `app/crates/protocol`, `app/crates/model`, `app/crates/transport`, `app/desktop/crates/daemon_supervisor`, read in full. Method: reading only. Cargo, clippy and the rules script were not run. Compared against `app/mobile/src-tauri/src` (`api_proxy*`, `ws_bridge*`, `hosts*`, `host_tokens`, `tls_pin`, `sidecar/*`), `docs/rust-style.md`, the checklist, the relay security review, the pane-layout intent spec, and the extension contract and host protocol (enhancement branch).

Paths are relative to `app/`. Line numbers are per file.

## Blockers

None that break the build. B1 and B2 below are listed as major because they are correctness or security holes, but the owner may treat them as gate blockers.

## Major

### M1. Supervisor stops draining the daemon's output on the first non-UTF-8 byte, so the daemon can wedge
`desktop/crates/daemon_supervisor/src/output_pump.rs:14`
- **What:** `while let Ok(Some(line)) = lines.next_line().await` ends on any `Err`. `tokio::io::Lines` returns `InvalidData` for a line that is not valid UTF-8, for example a truncated multibyte character in a Go panic dump or a binary log.
- **Why it matters:** The pump task ends and the pipe is never read again. After 64 KB of further output the daemon blocks in `write(2)`, and the watchdog then reports a wedge. The original `pump_output` used `String::from_utf8_lossy` on raw bytes and never stopped. This is a dropped rule.
- **Fix:** Read with `read_until(b'\n')` (or `split`) on bytes and convert with `from_utf8_lossy`. Cap the line length (see m12). Keep draining after any error except EOF, and log the error once at `warn`.

### M2. Probe loses the descriptor-exhaustion classification after the first dial
`desktop/crates/daemon_supervisor/src/probe.rs:32-48`, `health.rs:56-66`
- **What:** `probe_ready` classifies EMFILE/ENFILE only for its first, throwaway `UnixStream::connect`. `check_health` and `get` then connect again, call `.to_string()` on the error (`health.rs:59,62`) and return `Err(String)`, which `probe_ready` maps to `Probe::Unserved`.
- **Why it matters:** The original `probe_ready` walked the whole error chain of every dial (`dial_failure`). With the port, a process that runs out of file descriptors on the second or third dial counts the failure against a healthy daemon. Six in a row lead to SIGQUIT and SIGKILL. `Probe::LocalDescriptorExhaustion` exists to prevent exactly this. The redundant first dial also costs a connection per probe.
- **Fix:** Give `get` and `check_health` a typed error that keeps the `io::Error` (an `Error::Dial { source }` variant). Classify in one place. Delete the throwaway connect. Add a test that injects EMFILE on the second dial.

### M3. Relay extension scope is not stamped on WebSocket upstreams
`transport/src/upstream.rs:42-70`, `session.rs:33-57`, compare `transport.rs:100-110`
- **What:** `Transport::route` strips and sets `x-crowbar-extension` on HTTP. `upstream::open` builds the upgrade request with no such header and does not receive the `Admission.scope`.
- **Why it matters:** The security review (§3 and §4) and host protocol §8 promise that "the daemon enforces the same rule independently". For every stream opened with an extension token the daemon cannot tell it from a core stream, so the daemon-side check is impossible for WebSockets. The relay is then the only barrier on the extension path.
- **Fix:** Pass the `RelayScope` or `AccessScope` into `Transport::open_stream` and `upstream::open`, and insert the header (`stamp_scope` is reusable). Add a relay test that asserts the upgrade request the fake daemon receives carries the header for an extension token and not for a core token.

### M4. Revocation can be lost: the removals subscriber subscribes late and ignores lag
`transport/src/relay.rs:56,137-157`, `transport.rs:47,76-79`
- **What:** (a) `follow_removals` calls `removed_hosts()` (`subscribe()`) inside the spawned task. A `remove_host` that runs between `Relay::start` returning and the task's first poll is never seen, because a broadcast receiver only sees later sends. (b) `Err(RecvError::Lagged(_)) => {}` drops missed removals silently. The channel capacity is 16 (`transport.rs:47`).
- **Why it matters:** A removed host keeps live tokens and open streams, which defeats "removing a host closes its streams". The unit test `removing_a_host_cancels_its_open_streams...` (`relay_tests.rs`) runs `remove_host` synchronously straight after `relay_over(...)` on a current-thread runtime, so it only passes if something yields in between.
- **Fix:** Subscribe in `Relay::start` before `spawn` and move the receiver into the task. On `Lagged`, reconcile against the registry: revoke every token and close every stream whose host is no longer in `HostRegistry`. Add tests for both cases.

### M5. Handshake TOCTOU: a revoked pane can still end up with an open stream
`transport/src/gate.rs` (`inspect`, authorize), `session.rs:33-57` (register), `relay.rs:101-110` (`revoke_pane`)
- **What:** The token is validated in the handshake callback. The stream is registered in `ConnectionBook` only after the 101 is sent and the admission is received. `revoke_pane` or `remove_host` between the two removes the token and closes registered streams, but finds nothing to close. The late `register` then succeeds, and the stream lives until the page goes away.
- **Why it matters:** "A pane's destruction revokes its tokens and closes its open streams" is violated for in-flight handshakes.
- **Fix:** Register in the gate callback (before replying 101) so authorize and register are one critical section. Alternatively, re-check `tokens.authorize` inside `serve` after `register`. Add a test with a gate callback that blocks.

### M6. The close path has no deadline: a stuck client leaks the stream and its slot
`transport/src/bridge.rs:124-150` (`finish`), `:48-62` (`forward`)
- **What:** `finish` awaits `client.send(close)` and then `upstream.send(close)` with no timeout. `forward` awaits `write.send(message)` with no timeout. `next_within` applies the idle timeout to reads only.
- **Why it matters:** A pane that stops reading (a hung webview) fills its TCP window. On revoke, `client.send(close)` blocks forever and the upstream close is never sent. The task keeps the `ConnectionGuard` (the slot) and the daemon socket. 256 such streams exhaust the relay. This breaks the checklist's "closed on every path" rule.
- **Fix:** Wrap `finish` in `tokio::time::timeout(CLOSE_GRACE, ...)`. Close the upstream first, or concurrently. Bound each `write.send` with a write deadline, and treat a timeout as `Ending::Gone`. Test with a client that never reads.

### M7. `to_http` interpolates an unvalidated path into the URI authority
`transport/src/daemon_wire.rs:21-25`
- **What:** `format!("crowbar://{}{}", authority(&request.host), request.path_and_query)` is built from a free-form `String`. A `path_and_query` of `@host-2/v0/x` yields `crowbar://localhost@host-2/v0/x`. `uri.host()` is then `host-2`, `target_of` routes to the remote host `host-2`, and `scope.check` sees only `/v0/x` and passes. `-x/v0/x` appends to `localhost` and gives `localhost-x`.
- **Why it matters:** The caller's `host` claim (the thing a shell-side policy checks) differs from the host actually dialled. This is a boundary-validation failure under checklist §11.
- **Fix:** Require `path_and_query` to start with a single `/`, then parse it as `http::uri::PathAndQuery`. Build the URI with `Uri::builder().scheme("crowbar").authority(...).path_and_query(...)`, and assert that the parsed host equals the requested one. Add a test with `@`, `:`, `.` and `\` prefixes.

### M8. `RelayEndpoint` derives `Debug` and prints its token
`protocol/src/relay_endpoint.rs:4-11`
- **What:** `#[derive(Debug)]` on a struct with `pub token: String`. `Credential` redacts its token (`transport/src/credential.rs:17`), so the two are inconsistent.
- **Why it matters:** Any `{:?}` or `tracing` of the endpoint, `Result<RelayEndpoint>`, or an enclosing frame writes a live relay token to logs and panics. The security review says tokens are never logged.
- **Fix:** Make the token a `RelayToken` newtype with a redacting `Debug` (serde-transparent), or implement `Debug` by hand. Make the fields private.

### M9. Pairing leaves an orphaned credential when the registry write fails
`transport/src/pairing.rs:47-50`
- **What:** `keep_or_revoke` stores the token in the keychain. Then `registry.insert(...)?` can fail (disk full, permissions). The function returns `Err` with the credential kept in the keychain and the token still valid on the host. The doc comment says "on any failure the registry and keychain are untouched".
- **Why it matters:** A silent live credential for an unregistered host, and a false documented guarantee.
- **Fix:** Insert into the registry first (it is reversible), or on failure delete the keychain entry and run `revoke`. Add a test with a failing hosts file.

### M10. `remove_host` does not forget the host's credential
`transport/src/transport.rs:76-79`
- **What:** It removes the registry entry and broadcasts. Nothing calls `TokenStore::delete`. In the whole crate `delete` is only defined, never called. The original exposed `forget_host_token`.
- **Why it matters:** The bearer token and pin stay in the OS keychain, and a later pairing under the same id silently reuses the cache slot. Dropped behaviour.
- **Fix:** `remove_host` (or a `Transport::forget_host`) deletes the keychain entry on the blocking pool and surfaces the error. Order it so a failed delete is reported without leaving the host half-removed. Optionally revoke on the host as pairing does.

### M11. Daemon-supplied headers are dropped silently and bodies copied three times
`transport/src/daemon_wire.rs:55`, `:31`, `:60`; `http_conn.rs:91`
- **What:** `from_http` uses `filter_map(... value.to_str().ok()?)`, which drops every response header whose value is not visible ASCII. That includes UTF-8 `Content-Disposition` filenames and non-ASCII `Set-Cookie` values. The old path handed `http::Response` straight to the webview. `to_http` and `from_http` take `&`, so they clone the whole request and response body. `collect_capped` does `to_bytes().to_vec()`, which copies again (the local path has no cap, `local.rs:98`).
- **Why it matters:** A behaviour change from the original (lost headers) and a peak memory of roughly three times the body for `crowbar://` blob fetches. `DaemonResponse.body: Vec<u8>` also rules out the streaming that the pane-host plan promises for bulk data.
- **Fix:** Carry headers as `Vec<(String, Vec<u8>)>` or `HeaderValue`. Take the request and response by value. Use `Bytes` in `DaemonResponse` and the collected body. For the streaming gap, open a plan in `docs/plans/` rather than shipping a whole-body-only trait.

### M12. Public surface of `transport` is about 60 items with one external consumer
`transport/src/lib.rs:70-112`
- **What:** The only consumer outside the crate is `daemon_supervisor::health` (`local_get`). Everything else is re-exported: `dial`, `connect_retry_delay`, `CONNECT_ATTEMPTS`, `qualify`, `split_host_path`, `HostPath`, `hex`, `exchange`, `send`, `open_channel`, `collect_capped`, `paired_host`, `stored_credential`, `bearer`, `forward_local`, `forward_remote`, `pinned_client_config`, `Pin`, `Credential`, and others. `qualify`, `split_host_path` and `HostPath` have no caller at all.
- **Why it matters:** M13, M14, M15 and M37: the entry, `Error`, traits and signature types only, and delete what is unused. A public `exchange`/`send` also lets any caller bypass `AccessScope` and the header stamping. `unreachable_pub` cannot catch this because the items are re-exported.
- **Fix:** Re-export only what the shell and tests need: `Transport`, `ShellTransport`, `Relay`, `RelayConfig`, `StreamAccess`, `OriginPolicy`, `ExactOrigins`, `HostRegistry`, `TokenStore`, `CachedTokenStore`, `MemoryTokenStore` (test-support), `Keychain`, `OsKeychain`, `SocketLocator`, `AccessScope`, `Error`, and `pair_remote_host`/`probe_fingerprint`/`PairRequest`. Make the rest `pub(crate)`. Delete `host_path.rs` or add its consumer in the same change. Keep `local_get` only if the supervisor really needs it (see m14).

### M13. The browser-tab deny rule did not move
`transport/src/access_scope.rs:16-21`, original `api_proxy.rs` `serve()`
- **What:** The original refused every `crowbar://` request from a browser-tab webview (`exempt_from_app_guard`, 403) before any connection. `AccessScope` has only `Core` and `Extension`. There is no deny scope, and nothing in this crate (or in the plans for it) says where the rule lives now.
- **Why it matters:** "Remote pages in browser tabs must never reach a daemon API" was an invariant. Today it holds only if every future pane host remembers to call neither `handle` nor `relay_endpoint` for browser panes.
- **Fix:** Add `AccessScope::Denied` (or make `DaemonRequest.origin` a three-way kind) so the transport refuses it, and add a test. At minimum, state the owner of the rule in `docs/plans/2026-11-14-gpui-shell-pane-host.md`.

### M14. Contradictory scopes are representable (`RelayScope`, `DaemonRequest`)
`protocol/src/relay_scope.rs:8-17`, `daemon_request.rs:7-22`; consumers `access_scope.rs:28`, `exact_origins.rs:30`
- **What:** `kind: PaneKind` plus `extension: Option<ExtensionId>` allows `Core + Some` and `Extension + None`. The code rejects these at three places (`AccessScope::of`, `Relay::issue`, `ExactOrigins::allows`) after the fact.
- **Why it matters:** The pane-layout spec (item 4) says "make invalid states unrepresentable". All fields are `pub`, so any caller can build the bad value. This is a security-relevant scope.
- **Fix:** Use `enum PaneOrigin { Core, Extension(ExtensionId) }` in both types and drop `PaneKind` from them. `AccessScope` then becomes `PaneOrigin` (or a thin wrapper). Make the fields private with a constructor.

### M15. `PaneTypeId::extension()` can silently disagree with `kind()`
`protocol/src/pane_type_id.rs:6-24,35-48`, `extension_id.rs:4-15`
- **What:** `validate` allows `_` and a leading digit in any segment, so `ext.1foo.bar` and `ext.my_ext.bar` are valid. `kind()` returns `Extension`. `extension()` runs `ExtensionId::new(name).ok()`, which fails, and returns `None`.
- **Why it matters:** Code that maps "extension pane with no extension" to core defaults, or to the wrong scope, gets an extension pane without scope. `.ok()` hides the failure.
- **Fix:** In `validate`, require the second segment to pass `ExtensionId` validation. Make `extension()` infallible by construction, or return `Result`.

### M16. Relay accept loop: unauthenticated clients can occupy every slot
`transport/src/relay.rs:113-134`, `relay_config.rs:27`
- **What:** The "handshake semaphore" has `2 * max_connections` permits and each permit is held for the whole session (`drop(permit)` runs after `session::run`, which includes bridging). An overflow connection is dropped without a status (`:126-128`). Pre-auth clients get a 10 s `handshake_timeout`.
- **Why it matters:** Any local process, with no token, can open 512 idle TCP connections every 10 s and starve every legitimate pane. The review's "bounded by the semaphore" is true, but the bound is the DoS. The doc also claims handshake slots are separate from stream slots, which is false.
- **Fix:** Release the permit once the handshake finishes. Add a separate bounded counter for open streams. Shorten the pre-auth deadline (head received within about 2 s). Answer overflow with `503` and a log line. Add a test with `max_connections = 1`.

### M17. Layout: `apply` clones every pane's state on every command, twice for history
`model/src/layout.rs:53-57`, `layout_history.rs:51-59`
- **What:** `Layout::apply` clones the whole layout (all tabs, all panes, all `PaneState`s up to 64 KB each) before every command. `LayoutHistory::apply` clones again, including for non-structural commands (`before` is taken before `is_structural` is used). `SetPaneState` is the hot command: a pane can send it per keystroke.
- **Why it matters:** Checklist §8: no clone in per-keystroke paths. Twenty panes of 64 KB means about 2.5 MB of copying per keystroke.
- **Fix:** Take the history snapshot only when `structural`. For `SetPaneState`, `FocusPane`, `ActivateTab`, `ResizeSplit`, validate first and mutate in place, since they cannot fail midway. Keep clone-and-swap for the multi-step commands, or implement an undo log.

### M18. Undo stores 64 full copies including all pane state; no byte bound
`model/src/layout_history.rs:5,53,62-64`
- **What:** `MAX_UNDO = 64` snapshots, each a full `Layout`. Worst case is 64 times the sum of all pane states (about 80 MB for 20 full panes). `undo.remove(0)` on a `Vec`.
- **Why it matters:** Checklist §4: bounded with an eviction rule. Bounded by count only. State is also duplicated needlessly, because `adopt_states` overwrites it with the live copy anyway.
- **Fix:** Keep states out of the snapshot (arrangement only: tabs, trees, focus, ids). Hold the `PaneInstance`s of closed panes in one side map. Use a `VecDeque`.

### M19. Closing the focused pane reproduces the documented "focus goes to the far left" bug
`model/src/split_node.rs:106-115`, `tab.rs:140-153`; spec `pane-layout-intent.md` F4 bullet 1
- **What:** `sibling_of` returns `other.first_pane()`, the first leaf of the sibling subtree. For `[[A|B] | X]`, closing X focuses A, not the adjacent B. The unit test `sibling_is_the_first_pane_of_the_neighbouring_subtree` and the doc "pane that takes its space" lock this in.
- **Why it matters:** The spec's own example ("with three panes, closing the right one focuses the far left") is still true. There is also no MRU.
- **Fix:** Choose the leaf adjacent to the removed pane: the last leaf of a left or top sibling, the first leaf of a right or bottom sibling. Keep an MRU list in `Tab` and prefer it. Replace the test.

### M20. Last pane close, detach and merge need compound commands
`model/src/layout.rs:112,195-209`, `tab.rs:140`; spec F4, F5
- **What:** `ClosePane` on the last pane leaves an empty tab. The spec says closing the only chat of a tab closes the tab and shows the adjacent tab. "Open in its own tab" has no command either: `MovePaneToTab` needs an existing tab, so the caller sends `AddTab` and then `MovePaneToTab`.
- **Why it matters:** Two commands means two undo steps and a visible intermediate state. Spec F4 and F5 call out the same pattern ("two writes", "two undoable-less steps") as bugs.
- **Fix:** Add `Command::DetachPane { pane, new_tab }` and make `ClosePane` close an emptied tab, or add an explicit `ClosePaneAndEmptyTab`. State the choice in `command.rs` docs and test the undo as one step.

### M21. Supervisor has no `tests/support/mod.rs` conformance; `mod.rs` is denied
`daemon_supervisor/tests/support/mod.rs` (declared by `mod support;`, `tests/daemon_supervisor.rs:11`); `Cargo.toml` `mod_module_files = "deny"`; style M15
- **What:** A `mod.rs` file. Transport uses `#[path = "support/x.rs"]` instead.
- **Why it matters:** The `mod_module_files` lint and the `mod-rs` script reject it, so `cargo clippy --all-targets` and the rules script should fail. Not run here.
- **Fix:** Rename to `tests/support/daemon.rs` and use `#[path = "support/daemon.rs"] mod support;`.

## Minor

### m1. Spawned upstream upgrade for local streams has no deadline
`transport/src/upstream.rs:54` (`HostTarget::Local => upgrade.await`). Same as the original, but the relay now holds a stream slot and the client socket while it waits. `open_stream` is not raced against the guard's cancel token (`session.rs:47-57`), so a revoke during the upgrade is only noticed after it finishes. Fix: `select!` with `guard.cancelled()` and add a timeout.

### m2. Per-stream idle timeout dropped; defaults hide half-open streams
`transport/src/relay_config.rs:17-19,30-31`. The original `ws_open` took `idle_timeout_ms` per stream, for daemons that ping. The relay has one global pair that defaults to `None`. A global value would kill quiet streams that do not ping. Fix: carry the idle policy per path (a table of path prefix to idle) or in the token scope, and document the default.

### m3. Relay refuses every error with the reason `daemon-unavailable` except two
`transport/src/session.rs:59-68`. `NoCredential`, `UnknownHost`, `Forbidden`, `Random` all become the same close reason. The original returned `<marker>: <reason>` so a pane could prompt a re-pair. Fix: add markers for `no-credential` and `unknown-host` and map them in `Error::marker`.

### m4. WebSocket protocol errors are reported as a plain TCP close
`transport/src/bridge.rs:83`. `Ok(Some(Err(_)))` maps to `Ending::Gone`. A message over the cap or a protocol error is therefore not answered with close 1009 or 1002, and the reason is lost. Fix: carry the `WsError` kind in `Ending` and send the matching close code.

### m5. `relay_endpoint` stamps nothing about kind into the URL; tokens are never zeroized
`transport/src/credential.rs:12`, `token_book.rs`, `remote.rs:145-150`. `Credential.token` and the issued token are plain `String`s that live in clones. Checklist §11 asks for zeroization or short lifetime. Fix: wrap in `Zeroizing<String>` (or document why not) and avoid cloning the credential through `RemoteHost`.

### m6. `HostRegistry::change` holds the write lock across disk I/O
`transport/src/host_registry.rs:86-95`. `save` does `create_dir_all`, write and `sync_all` under `RwLock::write`, and every request's `registry.get` blocks behind it on an async worker. `write_atomically` (`hosts_document.rs:63-71`) uses a fixed `.tmp` name, no parent-directory fsync and no cleanup on failure. `Transport::remove_host` is a sync fn that does the I/O. Fix: write outside the lock (serialise writers with a separate mutex), fsync the directory, remove the temp file on error, and make `remove_host` async or document that it blocks.

### m7. Hosts file: duplicates collapse silently; a bad entry rejects the file
`transport/src/hosts_document.rs:34-37`. `collect::<Result<BTreeMap<_,_>>>` keeps the last duplicate id. One entry that fails `check_registrable` (for example a pre-existing `local`) fails the whole load. Fix: reject duplicates explicitly, and report which entry is wrong.

### m8. Host-id rule changed from the original without a note
`transport/src/host_target.rs:5,27-41`. `local` is now barred and routed to the local daemon. The original only barred `localhost` and the empty string. Anyone who paired a host named `local` is broken. `HostId` accepts `_`, but `check_registrable` rejects it, so the type admits ids the registry refuses. Fix: record the change in the migration plan, or introduce a `RemoteHostId` type that makes the rule structural.

### m9. `Endpoint::connect` tries addresses one by one inside a single 5 s budget
`transport/src/endpoint.rs:63-73` with `dial.rs` `DIAL_TIMEOUT`. A dead first address (common with dual-stack names) consumes the budget. Fix: race addresses with a short stagger (happy eyeballs), or divide the budget.

### m10. Pane-supplied hop-by-hop headers pass through
`transport/src/http_conn.rs:26-40`. `host`, `connection`, `upgrade`, `content-length`, `transfer-encoding` are copied to the daemon. One connection per request limits the impact, and the original did the same, but a proxy should strip them. `Authorization` is correctly overwritten for remote hosts only (`remote.rs`); for the local daemon a pane-sent `Authorization` is passed. Fix: strip the hop-by-hop set and set `Host` to a constant.

### m11. 503 body for a missing local daemon includes the home path
`transport/src/local.rs:42`, `:96`. `format!("crowbar daemon socket {}: {e}", path.display())` ends in the page-readable error body (CORS-granted by `readable_by`). Checklist §11: no home paths in error strings. Also `Error::LocalSocket(String)` (`error.rs:170`) drops the `io::Error` source (M9/M27). Fix: keep the source in the error, show a generic message to the page and log the path.

### m12. `.ok()` discards on results without a logged reason
`bridge.rs:55,58,...`, `http_conn.rs:21` (`connection.await.ok()` in a detached task), `transport.rs:78`, `supervise.rs:75,83,...`, `supervisor.rs:67`. Most are legitimate (peer gone) but M26 asks for the reason. Fix: `tracing::debug!` with the error in each, and use a named helper for "peer already gone".

### m13. `ShellTransport::request`: unbounded, uncancellable, detached
`transport/src/shell_transport.rs:54-59`. One `spawn` per request with no concurrency limit and no cancel handle; a navigating webview cannot cancel a 300 s proxy request. A panic in `answer` drops `respond` unanswered, which breaks the "exactly once" contract in `protocol/src/daemon_transport.rs:7`. The original's PLATFORM note (answer a cancelled WebKit task only on the main thread) is gone from the docs. Fix: a `Semaphore` per host, return a cancel handle, wrap in `catch_unwind`-free structure (call `respond` from a drop guard), and document the UI-thread hop in `DaemonTransport::request`.

### m14. Supervisor depends on the whole transport crate for one helper
`daemon_supervisor/Cargo.toml:27`, `src/health.rs:6`. Pulls rustls, tungstenite, keyring features and the relay into the supervisor for a 10-line GET. Fix: move `local_get`/`exchange` into a small `daemon_http` crate, or have the supervisor use `hyper` directly.

### m15. `Supervisor` drop docs are wrong; stop is not guaranteed at app exit
`daemon_supervisor/src/supervisor.rs:14-16` says dropping aborts the task and kills the child. Dropping a `JoinHandle` detaches. What really happens: the command sender closes, `recv()` returns `None`, and every phase treats that as a graceful stop. If the runtime is shut down first, `kill_on_drop` sends SIGKILL, with no SIGTERM and no state flush. Fix: correct the doc, implement `Drop` that cancels, and document that the shell must `await stop()` before exiting.

### m16. Stale watchdog can emit `Unhealthy` for a new daemon
`daemon_supervisor/src/watchdog.rs:86-98`. The cancel token is checked only at the sleep. `cycle` (up to about 13 s of probes) and `bus.emit(DaemonEvent::Unhealthy)` run after cancellation, so a watchdog of the previous incarnation can announce a wedge for the next healthy one. Fix: `select!` the whole cycle with `cancel.cancelled()` and re-check `is_cancelled()` before emitting.

### m17. Pid-mismatch warning dropped; respawn delay changed
`daemon_supervisor/src/supervise.rs:68`, `:211-216`. The original warned when `/v0/health` reported a pid other than the child's (a stale daemon on our socket). The port emits `Ready{pid: reported}` without comparing. Respawn went from a fixed 500 ms to 500 ms doubling to 10 s. Reasonable, but undocumented in the plan. Fix: compare and log; note the change in the migration plan.

### m18. Crash report lost the app version
`daemon_supervisor/src/crash_report.rs:41-46`, `daemon_event.rs:32` says "Versions". The original printed `Crowbar {version} on {os} {arch}`. Fix: add a `version: String` to `SupervisorConfig` and print it.

### m19. Supervisor creates the socket directory and log with default permissions
`daemon_supervisor/src/spawn_daemon.rs:11-16`, `daemon_log.rs:70`. `create_dir_all` and `OpenOptions` use the umask. The log can hold prompts, paths and stack traces. Fix: `DirBuilder` with mode `0o700` and `OpenOptions::mode(0o600)` (the same applies to `hosts.json`).

### m20. Log writing is synchronous inside async tasks; lines are unbounded
`daemon_supervisor/src/daemon_log.rs:42-53`, `output_pump.rs:14`. `writeln!` on a `std::fs::File` under a `std::sync::Mutex` runs on the runtime threads for every line. A line with no newline grows without limit. Rotation is checked by line count, not bytes (`ROTATE_CHECK_EVERY`). Fix: a bounded `mpsc` to one writer task using `spawn_blocking`, a line length cap, and a byte-based check.

### m21. `ps` is spawned by name without timeout or `kill_on_drop`
`daemon_supervisor/src/process_cpu.rs:13-24`. PATH lookup of `ps` and `.output()` with no deadline inside the watchdog cycle. A hung `ps` stalls the watchdog. Fix: absolute `/bin/ps`, `kill_on_drop(true)`, `timeout`.

### m22. Watchdog and stop handling during `kill_wedged`
`daemon_supervisor/src/supervise.rs:154-165,188-209`. While `kill_wedged` runs (up to `dump_timeout + sigquit_grace`, about 12 s by default) the monitor loop does not read `commands`, so a `stop()` waits. Fix: race the kill with `commands.recv()` or run it as a task with a cancel token.

### m23. Contract frames missing from `protocol`
`protocol/src/pane_frame.rs`, `shell_frame.rs`. The SDK contract (§2) lists `settings.get/onChange`, `theme`, `composer.attach/draft` and `attachment.open`; none has a frame. If these are deliberately later, say so in `lib.rs`. Also: `PaneFrame::Request.method`, `Subscribe.event`, `RunCommand.command` and `SetTitle.title` are unbounded strings, and `Params` (`params.rs:11`) says "values are ids only" but validates nothing (no key or value alphabet, no entry count). Fix: validated newtypes (`MethodName`, `EventName`, `CommandId`), a title length cap, and `Params` entry limits.

### m24. `ShellFrame::Response` can carry both or neither of `result` and `error`
`protocol/src/shell_frame.rs:35-44`. Also `result: Some(Value::Null)` round-trips to `None` because `Option<Value>` deserializes `null` as `None`, so a successful `null` result is indistinguishable from "absent". Fix: `enum Outcome { Ok(Value), Err(RpcError) }`, flattened, with a test for the `null` result.

### m25. Public fields on wire structs; `Error` carries strings
`protocol/src/daemon_request.rs`, `daemon_response.rs`, `relay_endpoint.rs`, `key_binding.rs`, `rpc_error.rs`, `extension_status.rs`; `protocol/src/error.rs:70-75` (`HostUnavailable(String)`, `Denied(String)`). Style says public types have private fields (C-STRUCT-PRIVATE). `Denied(String)` and `HostUnavailable(String)` drop the source; callers cannot match on the cause. Fix: constructors and getters, `#[non_exhaustive]` on structs that may grow, typed causes.

### m26. Dead public API
`protocol/src/lifecycle.rs:25` (`Lifecycle::as_str`, duplicates the serde names), `version.rs:5` (`SDK_VERSION`), transport `host_path.rs` (see M12). No in-repo caller. Delete or wire up in the same change (M37).

### m27. `model`: other points
- `model/src/tab.rs:165-170`: `debug_assert!(placed)` after the pane is already lifted from the tree. In release a failed placement would leave a pane in `panes` and not in the tree, and `apply` does not validate afterwards. The check precedes it, so it is unreachable today; restructure so nothing is lifted before success is certain, or return `Error::PaneNotFound`.
- `model/src/tab.rs:21`: `Tab` derives `Deserialize` publicly and unvalidated; only `Layout`'s deserializer validates. Keep `Tab` deserialization crate-private.
- `model/src/layout.rs:195-205`: `MovePaneToTab` to the same tab returns `Error::SamePane`, which names the wrong cause. Add `SameTab`.
- `layout_history.rs:53-58`: no-op structural commands (`MoveTab` to the same index, `RenameTab` to the same title, `MovePane` to its current place) push an undo entry. Compare before and after, and skip equal layouts.
- `model/src/split_node.rs:11,179-203`: the minimum is a ratio of 0.1 and `divide` has no sash and no pixel minimum. Spec F3 bugs 1 and 3 (one minimum in px, `(size - sash) * ratio`) are not implemented yet. If the layout crate owns this, say so in `split_node.rs`.
- `Command::is_structural` test (`command_tests.rs`) covers 7 of 13 variants.
- `model/src/layout.rs:80-87`: `Layout` deserialization ignores unknown fields and there is no schema version (the plan says the file is versioned).
- No lookup by `(type, params)`: spec F2 "an already-open chat is moved, not duplicated" cannot be enforced by callers without scanning every pane.
- Unbounded depth and pane count; recursion in every `SplitNode` method. Add a pane cap per tab.

### m28. Test quality
- Real TCP, unix sockets and `/bin/sh` children are used in unit tests with no `#[ignore]` (M41): `transport/src/relay_tests.rs`, `session_tests.rs`, `local_tests.rs`, `daemon_supervisor/src/stop_tests.rs` (a real `sleep 30` and `trap`), `running_tests.rs`.
- Tests that only exercise code: `transport/src/tls_config_tests.rs` (asserts `is_ok`), `upstream_tests.rs` (reads back a setter), parts of `os_keychain_tests.rs`.
- Real-time budgets in supervisor integration tests (20 ms watchdog, 50 to 100 ms probes) are load sensitive: `tests/daemon_supervisor.rs:95-130`, `watchdog_tests.rs:14-16`. Use `tokio::time::pause` with injected clocks.
- Temp directories and sockets are named by pid and never removed (`tests/support/mod.rs:23`, `spawn_daemon_tests.rs:9`, `probe_tests.rs:15`).
- Missing tests for every finding above that is a defect: M1 to M7, M9, M10, m4, m16.
- `model/tests/model.rs` is a decent random walk but asserts only count and validity: add focus and pane-identity properties, and a case that includes `MovePaneToTab` with a placement.

## Nits

- `model` and `daemon_supervisor` have no file named after the crate (M10): `model.rs` is missing (entry is `layout.rs`); the supervisor entry is `supervisor.rs`.
- `host_target.rs:7-9`: `#[derive]` comes before the `///` doc.
- `TabId(pub u64)` and `RequestId(pub u64)` expose their field.
- `transport` is not in `[workspace.dependencies]` although it is used by path (`daemon_supervisor/Cargo.toml:27`).
- `probe_when_idle` in `dial.rs:59` sets keepalive; the name says the opposite of what it does.
- Error text such as `"{0}"` for `Http(hyper::Error)` (`error.rs`) gives no context (M27).
- `Error` exposes `hyper`, `http` and `tungstenite` types in its variants (C-STABLE).

## Comparison with the originals

Dropped or changed, intended or not:

| Area | Original | Now | Verdict |
|---|---|---|---|
| Browser-tab guard | 403 for browser webviews | none in crate | dropped (M13) |
| `forget_host_token` | exists | not called anywhere | dropped (M10) |
| Probe descriptor classification | whole error chain on every dial | first dial only | regression (M2) |
| Daemon output capture | lossy UTF-8, never stops | stops on invalid UTF-8 | regression (M1) |
| Response headers | all copied | non-ASCII dropped; duplicates now kept | changed (M11) |
| Request header collapse | `insert` per header (duplicates lost) | `extend` (kept) | improved |
| `x-crowbar-extension` | n/a | HTTP only | gap (M3) |
| WS idle timeout | per stream from JS | global, default off | changed (m2) |
| WS error to page | `<marker>: <reason>` | marker or `daemon-unavailable` | reduced (m3) |
| Page-load retirement | `close_for_window` | revoke by pane | intended, needs a caller on reload |
| Host id `local` | allowed | barred, routed to local | changed (m8) |
| Respawn delay | fixed 500 ms | 500 ms doubling to 10 s | changed (m17) |
| Stale daemon pid warning | logged | dropped | dropped (m17) |
| Crash report version | included | missing | dropped (m18) |
| Timeouts: PROXY 300 s, DIAL 5 s, keepalive 15 s/5 s, remote cap 256 MiB, connect retries (40 x 25 ms, then 100 ms doubling to 1 s, 47 attempts, GET/HEAD only), probe 5 s/8 s, threshold 6, SIGTERM 3 s, SIGQUIT 2 s | as listed | identical | kept |
| TLS pin rules (TLS 1.3 only, leaf digest, `InvalidCertificate` means identity changed, token stored with pin, TOFU confirmed by truncated fingerprint then full digest) | as listed | identical | kept |
| Watchdog rules (suspension skip, descriptor exhaustion skip, held probe resets, grace probe, CPU progress, SIGQUIT then SIGKILL, restart budget 3 in 600 s) | as listed | identical | kept |

## Conformance notes

- Extension contract: 64 KB state cap is enforced on construction and on deserialize (`pane_state.rs`); frame cap 256 KB checked before parsing; `extension.event` and `extension.status` match the host protocol §8 (`extensionId`, optional `spaceId`, name, payload; statuses `running|unavailable|disabled|back`). Lifecycle names match. Error codes match §7 (PascalCase). Scoped tokens: scope is per pane and host and kind (M14 for representability, M3 for the daemon-side header).
- Layering: no `gpui`, `tauri`, `wry` or `gtk` in any of the four crates; `model` and `transport` do not depend on each other; the supervisor depends on a shared crate only. No `unsafe`, no `unwrap`/`expect`/`panic` outside tests, no narrative comments, all files under 500 lines, `*_tests.rs` wired for every file with functions.
- Layout state machine: invariants hold by construction (tree leaves equal pane keys, split nodes always have two children, one tab per pane id, focus and active tab exist) and `Layout::validate` checks them on load. `apply` is atomic by clone-and-swap. Buggy behaviours from `pane-layout-intent.md` not reproduced: id reuse across roles, stage-as-tab, bottom tree, fullscreen, legacy formats, first-leaf focus of the whole layout. Reproduced in part: focus on close (M19). Not yet covered: sash and pixel minimum, last-pane close, detach.

## Verdicts

| Crate | Verdict | Reason |
|---|---|---|
| protocol | fix first | M8, M14, M15 and the minor items; shapes are sound and small |
| model | fix first | M17, M18, M19, M20; the state machine itself is correct and well tested |
| transport | fix first | M3 to M7, M9 to M13, M16; the pinned-TLS and token-book core is solid, no redesign needed, but the relay lifecycle (register, revoke, close) needs the fixes above before extensions ship |
| daemon_supervisor | fix first | M1, M2, M21; recovery and watchdog rules otherwise match the original |
