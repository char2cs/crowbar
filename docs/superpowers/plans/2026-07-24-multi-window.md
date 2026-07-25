# Multi-Window Crowbar Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let the user open a second identical Crowbar window on a different workspace, sharing one daemon, so two workspaces' agent chats are visible at once.

**Architecture:** Each Tauri window is its own JS realm, so the frontend isolates for free. The work is confined to four pieces of app-global Rust state that currently assume a single window: the two transport managers' `close_all()`, the sidecar teardown on window close, and the app-wide `terminal:transport-dropped` emit. Transport entries gain a `window_label` tag (IDs are already globally unique, so this is tagging, not re-keying), and teardown becomes a `retain()` scoped to the loading/closing window. A new `open_window` command builds additional windows, seeding the target route via a per-window init script that sets the URL hash.

**Tech Stack:** Rust / Tauri 2.11.5. **No frontend changes** — the routing this relies on (hash history) already exists.

**Spec:** `docs/superpowers/specs/2026-07-24-multi-window-design.md`

## Global Constraints

- **This branch has a concurrent agent session writing to it.** Only touch files named in your task. **Never `git add -A` or `git add .`** — always `git add <explicit paths>`. `desktop/src-tauri/src/lib.rs` has an unrelated uncommitted hunk (the `tauri_plugin_log` level tuning, ~line 485) that is NOT yours: never revert it, never stage it.
- Rust tests: `cd desktop/src-tauri && cargo test`. Lint: `cd desktop/src-tauri && cargo fmt --check && cargo clippy -- -D warnings`. Both must pass before every commit.
- **Every file you touch is under `desktop/src-tauri/src/`.** Nothing under `web/` or `api/` changes — this feature needs zero frontend and zero daemon changes. If you are editing outside `desktop/src-tauri/src/`, stop: you have left the plan.
- `close_all()` is **replaced** by `close_for_window()`, not kept alongside it. Leaving the app-global version in place invites accidental use.
- Commit after each task with the exact paths that task touched. Do not push, do not open a PR — commit locally and stop.

**Task order:** Tasks 1 and 2 are independent (different files, safe to run in parallel). Task 3 consumes both. Task 4 touches the same file as Task 3, so it must follow it.

---

### Task 1: Window-scope the WebSocket bridge

**Files:**
- Modify: `desktop/src-tauri/src/ws_bridge.rs`

**Interfaces:**
- Consumes: nothing from earlier tasks.
- Produces:
  - `WsBridgeManager::close_for_window(&self, label: &str)` — replaces `close_all()`.
  - `open_bridge<S: FrameSink>(socket: &Path, conn_id: String, path: String, on_message: S, manager: &WsBridgeManager, window_label: String) -> Result<tokio::task::JoinHandle<()>, String>` — `window_label` is the **last** parameter.
  - `ws_open` command gains `window: tauri::WebviewWindow` (Tauri injects it; the frontend passes nothing new).

- [ ] **Step 1: Add the multi-connection test daemon helper**

The existing `spawn_wedged_daemon` accepts exactly one connection. This test needs two on one listener. Add alongside it in the `tests` module (after `spawn_wedged_daemon`, around line 354):

```rust
    /// Same wedged peer as [`spawn_wedged_daemon`], but accepting connections forever
    /// rather than one. A window-scoping test needs two live connections on one
    /// listener; with the single-accept version the second `open_bridge` would block on
    /// a peer that never accepts.
    fn spawn_wedged_daemon_multi(listener: UnixListener) {
        tokio::spawn(async move {
            loop {
                match listener.accept().await {
                    Ok((stream, _)) => {
                        tokio::spawn(async move {
                            if let Ok(_ws) = tokio_tungstenite::accept_async(stream).await {
                                std::future::pending::<()>().await;
                            }
                        });
                    }
                    Err(_) => break,
                }
            }
        });
    }
```

- [ ] **Step 2: Write the failing test**

Add at the end of the `tests` module, after `close_all_retires_connections_a_reloaded_page_abandoned`:

```rust
    /// The multi-window invariant: a page load in ONE window must not touch another
    /// window's connections. Before window scoping this was `close_all`, so window B
    /// merely opening — or reloading — silently stranded every socket window A owned.
    #[tokio::test]
    async fn a_page_load_retires_only_the_loading_windows_connections() {
        let _serialised = crate::test_support::fd_tests().await;

        let sock = test_socket("window-scope");
        spawn_wedged_daemon_multi(UnixListener::bind(&sock).unwrap());
        let manager = WsBridgeManager::new();

        let main_reader = open_bridge(
            &sock,
            "main-conn".to_string(),
            "/v0/x".to_string(),
            silent_sink(),
            &manager,
            "main".to_string(),
        )
        .await
        .unwrap();

        let w2_reader = open_bridge(
            &sock,
            "w2-conn".to_string(),
            "/v0/x".to_string(),
            silent_sink(),
            &manager,
            "w2".to_string(),
        )
        .await
        .unwrap();

        manager.close_for_window("w2");

        w2_reader
            .await
            .expect("the loading window's own reader must end");

        {
            let conns = manager.connections.lock().unwrap();
            assert!(
                conns.contains_key("main-conn"),
                "window `main`'s connection must survive window `w2`'s page load"
            );
            assert!(
                !conns.contains_key("w2-conn"),
                "window `w2`'s own connection must be retired by its page load"
            );
        }

        // Clean up the surviving connection so the test leaves no live socket behind.
        manager.close_for_window("main");
        main_reader.await.expect("teardown must end the reader");

        let _ = std::fs::remove_file(&sock);
    }
```

- [ ] **Step 3: Run the test to verify it fails**

Run: `cd desktop/src-tauri && cargo test a_page_load_retires_only_the_loading_windows_connections`
Expected: FAIL to **compile** — `no method named close_for_window`, and `open_bridge` takes 5 arguments but 6 were supplied. Compilation failure is the correct "red" here.

- [ ] **Step 4: Tag connections with their window**

In `struct Connection` (line 83), add the field:

```rust
struct Connection {
    generation: u64,
    /// The Tauri window label that opened this connection. A page load retires only
    /// the connections belonging to the window that loaded — see `close_for_window`.
    window_label: String,
    tx: mpsc::UnboundedSender<Message>,
    /// Held, never used. Its `Drop` is the signal.
    _cancel: oneshot::Sender<()>,
}
```

- [ ] **Step 5: Replace `close_all` with `close_for_window`**

Replace the whole `close_all` method (lines 113-118):

```rust
    /// Retires every connection belonging to `label`. Called when a page load orphans
    /// that window's connections: the JS that owned these ids is gone and will never
    /// call `ws_close` for them, and the new page opens fresh ids of its own.
    ///
    /// Scoped to one window, never app-wide: with two windows open, an app-wide clear
    /// means window B merely reloading strands every socket window A owns. Connection
    /// ids are globally unique (`crypto.randomUUID` on the JS side), so the id-keyed
    /// map needs no re-keying — only teardown is window-aware.
    pub fn close_for_window(&self, label: &str) {
        self.connections
            .lock()
            .unwrap()
            .retain(|_, c| c.window_label != label);
    }
```

- [ ] **Step 6: Thread the label through `open_bridge` and `ws_open`**

In `open_bridge` (line 144), add `window_label: String` as the last parameter:

```rust
pub async fn open_bridge<S: FrameSink>(
    socket: &Path,
    conn_id: String,
    path: String,
    on_message: S,
    manager: &WsBridgeManager,
    window_label: String,
) -> Result<tokio::task::JoinHandle<()>, String> {
```

and store it in the inserted `Connection` (line 187):

```rust
    manager.connections.lock().unwrap().insert(
        conn_id.clone(),
        Connection {
            generation,
            window_label,
            tx,
            _cancel: cancel,
        },
    );
```

Then update the `ws_open` command (line 124):

```rust
#[tauri::command]
pub async fn ws_open(
    conn_id: String,
    path: String,
    on_message: Channel<String>,
    manager: State<'_, WsBridgeManager>,
    window: tauri::WebviewWindow,
) -> Result<(), String> {
    let socket = crate::sidecar::socket_path();
    open_bridge(
        &socket,
        conn_id,
        path,
        on_message,
        &manager,
        window.label().to_string(),
    )
    .await
    .map(|_reader| ())
}
```

- [ ] **Step 7: Update the three other test call sites**

`open_bridge` is called at lines 365, 407, 453 and 484 in the tests module. Add `"main".to_string()` as the final argument to each. In the test at line 484 (`close_all_retires_connections_a_reloaded_page_abandoned`), also rename the call `manager.close_all()` to `manager.close_for_window("main")` and rename the test itself to `a_page_load_retires_the_connections_a_reloaded_page_abandoned`, updating its doc comment's `close_all` reference to `close_for_window`.

- [ ] **Step 8: Run the full ws_bridge suite**

Run: `cd desktop/src-tauri && cargo test ws_bridge`
Expected: PASS — every test in the module, including the new one.

- [ ] **Step 9: Lint**

Run: `cd desktop/src-tauri && cargo fmt && cargo clippy -- -D warnings`
Expected: clippy clean, no warnings.

- [ ] **Step 10: Commit**

```bash
git add desktop/src-tauri/src/ws_bridge.rs
git commit -m "refactor(desktop): scope ws bridge teardown to the loading window"
```

---

### Task 2: Window-scope the terminal transport

**Files:**
- Modify: `desktop/src-tauri/src/terminal.rs`

**Interfaces:**
- Consumes: nothing from Task 1 (independent file; both are prerequisites for Task 3).
- Produces:
  - `TerminalManager::close_for_window(&self, label: &str)` — replaces `close_all()`.
  - `open_terminal_bridge` gains `window_label: String` as the parameter **immediately after `manager`** (before `idle_timeout`).
  - `terminal_open` command gains `window: tauri::WebviewWindow`.

- [ ] **Step 1: Add the multi-connection PTY test daemon helper**

`spawn_idle_pty_daemon` (line 420) accepts exactly one connection, so the two-window test would hang on its second `open_terminal_bridge`. Add alongside it:

```rust
    /// Same idle PTY peer as [`spawn_idle_pty_daemon`], but accepting connections
    /// forever rather than one. A window-scoping test needs two live sessions on one
    /// listener; with the single-accept version the second open would block on a peer
    /// that never accepts.
    fn spawn_idle_pty_daemon_multi(listener: UnixListener) {
        tokio::spawn(async move {
            loop {
                match listener.accept().await {
                    Ok((stream, _)) => {
                        tokio::spawn(async move {
                            if let Ok(_ws) = tokio_tungstenite::accept_async(stream).await {
                                std::future::pending::<()>().await;
                            }
                        });
                    }
                    Err(_) => break,
                }
            }
        });
    }
```

- [ ] **Step 2: Write the failing test**

Add at the end of the `tests` module in `terminal.rs`, after `close_all_releases_terminals_a_reloaded_page_abandoned`.

```rust
    /// The multi-window invariant: a page load in ONE window must not retire another
    /// window's terminals. Before window scoping this was `close_all`, so window B
    /// reloading killed every PTY transport window A had attached — the daemon keeps
    /// the PTYs alive, but window A goes permanently silent with no reattach.
    #[tokio::test]
    async fn a_page_load_retires_only_the_loading_windows_terminals() {
        let _serialised = crate::test_support::fd_tests().await;

        let sock = test_socket("terminal-window-scope");
        spawn_idle_pty_daemon_multi(UnixListener::bind(&sock).unwrap());
        let manager = TerminalManager::new();

        let main_reader = open_terminal_bridge(
            &sock,
            "main-session".to_string(),
            "/v0/x/ws".to_string(),
            silent_sink(),
            &manager,
            "main".to_string(),
            READ_IDLE_TIMEOUT,
            |_| {},
        )
        .await
        .unwrap();

        let w2_reader = open_terminal_bridge(
            &sock,
            "w2-session".to_string(),
            "/v0/x/ws".to_string(),
            silent_sink(),
            &manager,
            "w2".to_string(),
            READ_IDLE_TIMEOUT,
            |_| {},
        )
        .await
        .unwrap();

        manager.close_for_window("w2");

        w2_reader
            .await
            .expect("the loading window's own reader must end");

        {
            let sessions = manager.sessions.lock().unwrap();
            assert!(
                sessions.contains_key("main-session"),
                "window `main`'s terminal must survive window `w2`'s page load"
            );
            assert!(
                !sessions.contains_key("w2-session"),
                "window `w2`'s own terminal must be retired by its page load"
            );
        }

        manager.close_for_window("main");
        main_reader.await.expect("teardown must end the reader");

        let _ = std::fs::remove_file(&sock);
    }
```

- [ ] **Step 3: Run the test to verify it fails**

Run: `cd desktop/src-tauri && cargo test a_page_load_retires_only_the_loading_windows_terminals`
Expected: FAIL to compile — `no method named close_for_window`, wrong arity on `open_terminal_bridge`.

- [ ] **Step 4: Tag streaming legs with their window**

In `struct Streaming` (line 56):

```rust
struct Streaming {
    generation: u64,
    /// The Tauri window label that opened this streaming leg. A page load retires only
    /// the legs belonging to the window that loaded — see `close_for_window`.
    window_label: String,
    tx: mpsc::UnboundedSender<Message>,
    /// Held, never used. Its `Drop` is the signal.
    _cancel: oneshot::Sender<()>,
}
```

- [ ] **Step 5: Thread the label through `register` and replace `close_all`**

`register` (line 93) gains the parameter and stores it:

```rust
    fn register(
        &self,
        session_id: String,
        window_label: String,
        tx: mpsc::UnboundedSender<Message>,
        cancel: oneshot::Sender<()>,
    ) -> u64 {
        let generation = self
            .next_generation
            .fetch_add(1, std::sync::atomic::Ordering::Relaxed);
        // Dropping a previous entry retires it in full: its writer ends, its reader is
        // cancelled, and the old WS closes so the daemon detaches the stale client.
        self.sessions.lock().unwrap().insert(
            session_id,
            Streaming {
                generation,
                window_label,
                tx,
                _cancel: cancel,
            },
        );
        generation
    }
```

Replace `close_all` (lines 115-121) with:

```rust
    /// Retires every streaming leg belonging to `label`. Called when a page load orphans
    /// that window's sessions: the JS that owned them is gone and will never close them,
    /// and the reloaded page re-attaches the ones it still wants. The daemon keeps each
    /// PTY alive across this — only the streaming leg is torn down, never the session.
    ///
    /// Scoped to one window, never app-wide: with two windows open, an app-wide clear
    /// means window B merely reloading silences every terminal window A has attached.
    pub fn close_for_window(&self, label: &str) {
        self.sessions
            .lock()
            .unwrap()
            .retain(|_, s| s.window_label != label);
    }
```

- [ ] **Step 6: Thread the label through `open_terminal_bridge`**

Add `window_label: String` immediately after the `manager` parameter, and pass it to `register` at the call site (line 233, `let generation = manager.register(session_id.clone(), tx, cancel);` becomes `let generation = manager.register(session_id.clone(), window_label, tx, cancel);`).

- [ ] **Step 7: Target the transport-dropped emit at the owning window**

Rewrite `terminal_open` (line 146). `Emitter` is already imported (line 28); `emit_to` comes from the same trait.

```rust
#[tauri::command]
pub async fn terminal_open(
    session_id: String,
    // §3: the hierarchical PTY WS path, e.g.
    // /v0/projects/:p/repos/:r/workspaces/:w/terminals/:sessionId/ws. The
    // frontend builds it (workspace-scope aware) and hands it down so Rust no
    // longer hardcodes the removed flat /v0/ws/terminals/:id route.
    ws_path: String,
    on_data: Channel<String>,
    manager: State<'_, TerminalManager>,
    app: tauri::AppHandle,
    window: tauri::WebviewWindow,
) -> Result<(), String> {
    let socket = crate::sidecar::socket_path();
    let label = window.label().to_string();
    // emit_to, not emit: with two windows open an app-wide emit hands window A the
    // connection id of a transport window B dropped, and window A would try to
    // re-attach a session it does not own.
    let emit_label = label.clone();
    open_terminal_bridge(
        &socket,
        session_id,
        ws_path,
        on_data,
        &manager,
        label,
        READ_IDLE_TIMEOUT,
        move |dropped| {
            let _ = app.emit_to(&emit_label, "terminal:transport-dropped", dropped);
        },
    )
    .await
    .map(|_reader| ())
}
```

- [ ] **Step 8: Update the three other test call sites**

`open_terminal_bridge` is called at lines 450, 485, 517 and 561 in the tests module. Insert `"main".to_string()` after the `&manager` argument in each. In `close_all_releases_terminals_a_reloaded_page_abandoned` (line 478), change `manager.close_all()` to `manager.close_for_window("main")`, rename the test to `a_page_load_releases_the_terminals_a_reloaded_page_abandoned`, and update its doc comment's `close_all` reference.

- [ ] **Step 9: Run the full terminal suite**

Run: `cd desktop/src-tauri && cargo test terminal`
Expected: PASS — all tests including the new one and the generation-race test at line 603.

- [ ] **Step 10: Lint**

Run: `cd desktop/src-tauri && cargo fmt && cargo clippy -- -D warnings`
Expected: clean.

- [ ] **Step 11: Commit**

```bash
git add desktop/src-tauri/src/terminal.rs
git commit -m "refactor(desktop): scope terminal teardown and drop events to their window"
```

---

### Task 3: Scope page-load teardown and daemon lifetime

**Files:**
- Modify: `desktop/src-tauri/src/lib.rs` (`on_page_load` ~line 540, `on_window_event` ~line 604)

**Interfaces:**
- Consumes: `WsBridgeManager::close_for_window` (Task 1), `TerminalManager::close_for_window` (Task 2).
- Produces: nothing consumed by later tasks.

**Warning:** `lib.rs` carries an unrelated uncommitted hunk near line 485 (`tauri_plugin_log` level tuning). It is not yours. Do not touch or stage it — `git add` this file only after confirming with `git diff desktop/src-tauri/src/lib.rs` that your hunks are the only ones you introduced, and accept that the log hunk will ride along in the commit only if it is already staged by its owner. If it is unstaged, use `git add -p` to stage only your hunks.

- [ ] **Step 1: Scope the page-load teardown to the loading webview**

Replace the `on_page_load` body (lines 540-547):

```rust
        .on_page_load(|webview, payload| {
            if payload.event() != tauri::webview::PageLoadEvent::Started {
                return;
            }
            // Scoped to the webview that actually loaded, never app-wide: with two
            // windows open, an app-wide teardown means window B merely reloading
            // strands every socket and silences every terminal window A owns.
            let label = webview.label();
            let app = webview.app_handle();
            app.state::<ws_bridge::WsBridgeManager>()
                .close_for_window(label);
            app.state::<terminal::TerminalManager>()
                .close_for_window(label);
        })
```

- [ ] **Step 2: Guard the daemon teardown on the last window**

At the top of the `on_window_event` handler body (line 604), immediately inside the `if let ... = event {` block and before `if let Some(state) = window.try_state::<sidecar::SidecarHandle>()`, insert:

> **⚠️ This step's original code was WRONG and was corrected during
> implementation. `git show 1452ebd8` and `git show <teardown fix>` are the
> authoritative versions — do not restore what this box replaces.**
>
> The original read `if window.app_handle().webview_windows().len() > 1 {
> return; }`, justified by "`webview_windows()` still includes the closing window
> at CloseRequested time". That premise is only half true, and the handler
> matches **both** `CloseRequested` and `Destroyed`: `AppManager::on_window_close`
> prunes the map, which never runs for `CloseRequested` (window still counted)
> but runs synchronously *before* per-window listeners for `Destroyed` (window
> already gone). So with A and B open, `Destroyed(B)` sees `{A}`, `1 > 1` is
> false, and the daemon dies under A — the exact bug the guard exists to prevent.
> Verified against the pinned tauri 2.11.2 sources.
>
> A second omission was found in review: nothing retired the closing window's
> transports. `on_page_load` cannot cover a window that closes, because it never
> loads a page again — so its connections would park reader tasks on sockets for
> the life of the process, against a 256-descriptor ceiling this app really hits.

Insert, immediately inside the `if let ... = event {` block and before `if let Some(state) = window.try_state::<sidecar::SidecarHandle>()`:

```rust
                let label = window.label();

                // Retire this window's transports FIRST, whether or not it is the last
                // window — a closing window orphans its connections exactly as a
                // reloading one does, and on_page_load cannot cover it. Idempotent, so
                // the second pass (CloseRequested then Destroyed) is a no-op.
                let app = window.app_handle();
                app.state::<ws_bridge::WsBridgeManager>()
                    .close_for_window(label);
                app.state::<terminal::TerminalManager>()
                    .close_for_window(label);

                // Only the LAST window closing may stop the app-wide sidecar. Filter by
                // label, never by webview_windows().len(): see the warning above.
                let other_window_open = app
                    .webview_windows()
                    .keys()
                    .any(|other_label| other_label != label);
                if other_window_open {
                    return;
                }
```

- [ ] **Step 3: Build and run the whole suite**

Run: `cd desktop/src-tauri && cargo test`
Expected: PASS, and the crate compiles — this is the first point at which both managers' new signatures are exercised from `lib.rs`.

- [ ] **Step 4: Lint**

Run: `cd desktop/src-tauri && cargo fmt && cargo clippy -- -D warnings`
Expected: clean.

- [ ] **Step 5: Commit**

```bash
git add desktop/src-tauri/src/lib.rs
git commit -m "fix(desktop): scope page-load teardown and daemon shutdown per window"
```

---

### Task 4: The `open_window` command and menu item

**Files:**
- Modify: `desktop/src-tauri/src/lib.rs` (extract `decorate_window`, add `open_window`, extend `build_app_menu`, register the command)

**Interfaces:**
- Consumes: nothing.
- Produces: Tauri command `open_window` taking `{ route: String }` — the frontend invokes it as `invoke('open_window', { route })`.

- [ ] **Step 1: Extract the macOS window decoration into a reusable function**

The `setup()` closure currently applies vibrancy and the WebKit 60fps un-cap only to `get_webview_window("main")` (lines 566-590). Move that block verbatim into a free function placed just above `pub fn run()`:

```rust
/// Native macOS chrome for a Crowbar window: the vibrancy blur behind the transparent
/// window plus the post-creation WebKit 60fps un-cap. Applied to EVERY window, not just
/// `main` — a second window is the same app and must look and animate identically.
///
/// NSVisualEffectView blur is fixed per material (no numeric radius). `HudWindow` maps to
/// NSVisualEffectMaterialHUDWindow — a heavy, smooth blur. Requires `transparent: true` +
/// `macOSPrivateApi: true` (set in tauri.conf.json and mirrored in `open_window`).
#[cfg(target_os = "macos")]
fn decorate_window(window: &tauri::WebviewWindow) {
    use window_vibrancy::{apply_vibrancy, NSVisualEffectMaterial, NSVisualEffectState};

    if let Err(e) = apply_vibrancy(
        window,
        NSVisualEffectMaterial::HudWindow,
        Some(NSVisualEffectState::FollowsWindowActiveState),
        None,
    ) {
        log::error!("failed to apply window vibrancy: {e}");
    }

    // Step 2: post-creation setter, guarded by respondsToSelector:.
    let _ = window.with_webview(|wv| {
        use objc2::runtime::AnyObject;
        let ptr = wv.inner() as *mut AnyObject;
        unsafe { disable_webkit_60fps_cap_post(ptr) };
    });
}

#[cfg(not(target_os = "macos"))]
fn decorate_window(_window: &tauri::WebviewWindow) {}
```

Then replace the block in `setup()` with:

```rust
            #[cfg(target_os = "macos")]
            if let Some(window) = app.get_webview_window("main") {
                decorate_window(&window);
            }
```

- [ ] **Step 2: Add the `open_window` command**

Place it directly after `decorate_window`:

```rust
/// Opens another Crowbar window at `route` (a router path such as
/// `/ide/<projectId>/<repoId>/<wsId>`, or `/` for the picker).
///
/// Every window is the same app: same sidebar, same everything. What differs is only
/// which workspace it is looking at, and that is pure routing — the frontend uses hash
/// history, so the target is delivered by seeding `location.hash` from an init script
/// that runs before any app JS. The emptiness guard means the route applies on first
/// load only, so a later reload keeps wherever the user has since navigated instead of
/// yanking them back here.
///
/// The window options must mirror tauri.conf.json's `app.windows[0]`: that config
/// describes only the FIRST window, and a builder-made window inherits none of it.
#[tauri::command]
async fn open_window(app: tauri::AppHandle, route: String) -> Result<(), String> {
    use std::sync::atomic::{AtomicU64, Ordering};
    static NEXT_WINDOW: AtomicU64 = AtomicU64::new(2);

    let label = format!("w{}", NEXT_WINDOW.fetch_add(1, Ordering::Relaxed));
    let seed_hash = format!(
        "if (!location.hash || location.hash === '#/') {{ location.hash = {}; }}",
        serde_json::to_string(&route).map_err(|e| format!("encode route: {e}"))?
    );

    let mut builder = tauri::WebviewWindowBuilder::new(
        &app,
        &label,
        tauri::WebviewUrl::App("index.html".into()),
    )
    .title("Crowbar")
    .inner_size(1200.0, 800.0)
    .resizable(true)
    .transparent(true)
    .background_throttling(tauri::utils::config::BackgroundThrottlingPolicy::Disabled)
    .initialization_script(&seed_hash);

    #[cfg(target_os = "macos")]
    {
        builder = builder
            .title_bar_style(tauri::TitleBarStyle::Overlay)
            .hidden_title(true)
            .traffic_light_position(tauri::LogicalPosition::new(12.0, 23.0));
    }

    let window = builder.build().map_err(|e| {
        log::error!("open_window: build failed: {e}");
        format!("open window: {e}")
    })?;

    decorate_window(&window);
    Ok(())
}
```

- [ ] **Step 3: Register the command**

In the `invoke_handler` list (line 658), add `open_window,` after `set_vibrancy_appearance,`.

- [ ] **Step 4: Add the menu item**

Replace the `window_menu` line (line 334) in `build_app_menu`:

```rust
    // No close_window(): Cmd+W is freed for the in-app close-active-tab binding.
    // New Window is Cmd+Shift+N, not Cmd+N: Cmd+N is unclaimed today but reads as
    // "new file", and the in-app new-tab binding is already Cmd+T.
    let new_window = tauri::menu::MenuItemBuilder::new("New Window")
        .id("new_window")
        .accelerator("CmdOrCtrl+Shift+N")
        .build(app)?;
    let window_menu = SubmenuBuilder::new(app, "Window")
        .item(&new_window)
        .minimize()
        .build()?;
```

- [ ] **Step 5: Handle the menu event**

The builder currently has no `.on_menu_event`. Add it immediately after the `.menu(build_app_menu)` registration (around line 526), inside the same `#[cfg(target_os = "macos")]` block:

```rust
        builder = builder.on_menu_event(|app, event| {
            if event.id() == "new_window" {
                let app = app.clone();
                // The command is async and menu events are sync: spawn so the menu
                // handler returns immediately rather than blocking the event loop.
                tauri::async_runtime::spawn(async move {
                    if let Err(e) = open_window(app, "/".to_string()).await {
                        log::error!("New Window menu item failed: {e}");
                    }
                });
            }
        });
```

- [ ] **Step 6: Build and test**

Run: `cd desktop/src-tauri && cargo test`
Expected: PASS and compiles.

Every API used above was verified against the resolved dependency, **tauri 2.11.5**, before this plan was written — `traffic_light_position<P: Into<Position>>` (`webview/webview_window.rs:736`, macOS-gated, and it documents "Requires titleBarStyle: Overlay", which is why the `#[cfg]` block sets both), `background_throttling(BackgroundThrottlingPolicy)` (`:1204`), `title_bar_style` (`:726`), `hidden_title` (`:763`), `initialization_script(impl Into<String>)` (`:945`), `Builder::on_menu_event` (`app.rs:2000`), `Emitter::emit_to` (`lib.rs:981`), `Manager::webview_windows` (`lib.rs:588`), and `From<LogicalPosition<P>> for Position` (`dpi-0.1.2/lib.rs:776`, with `LogicalPosition` re-exported from `tauri`). If something still does not resolve, re-check against that path rather than guessing, and note the deviation in the commit message.

- [ ] **Step 7: Lint**

Run: `cd desktop/src-tauri && cargo fmt && cargo clippy -- -D warnings`
Expected: clean.

- [ ] **Step 8: Commit**

```bash
git add desktop/src-tauri/src/lib.rs
git commit -m "feat(desktop): add open_window command and New Window menu item"
```

---

### Cut from v1: the "Open in New Window" row affordance

Specified, then cut during plan review. **Do not implement it.** Workspace rows
have no context menu today (`workspace-tree-item.tsx` has no menu primitive;
`PlaceholderRowActions` is a special-case detach button), so adding one means a
new UI pattern in the sidebar — sensitive surface — to save a single click that
`New Window` + clicking the workspace already covers. The `open_window` command
takes an arbitrary route, so nothing blocks adding this later.

**There is no frontend work in this plan.** If you find yourself editing
anything under `web/`, stop: you have gone outside the plan.

---

## Final gate: live verification (NOT a subagent task)

Unit tests cannot certify multi-window macOS behaviour. Run this in the real app
before any completion claim.

- [ ] Reuse the already-running dev instance — do not spawn a second one. Check first with `pgrep -fl crowbar-api`; `/Applications` is prod, `worktree/target/debug` is dev. If no dev instance is running, `make dev-desktop`.
- [ ] Open window B via Cmd+Shift+N, navigate it to a **different** workspace than window A.
- [ ] Start an agent chat in each window. Confirm both stream output **simultaneously**.
- [ ] **Reload window B (Cmd+R).** Window A's terminals and agent chats must keep streaming. This is the decisive test — it is what `close_all` broke.
- [ ] Close window B. Window A must stay fully live: daemon up, chats still streaming.
- [ ] Confirm window B has the vibrancy blur and correctly positioned traffic lights.
- [ ] Confirm no `terminal:transport-dropped` cross-talk: reloading B must not disturb A's terminals.

If any step fails, that is a real defect — fix it before reporting completion.
