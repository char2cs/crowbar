# GPUI shell: pane host and daemon transport

Status: DRAFT for owner review. Frame encoding is not final; it is agreed with the contract agent before anything builds on it.

## 1. What a spike proved (Linux/X11, Docker, GPUI 0.2.2, wry 0.53)

- A GPUI window hosts a wry child webview; the real Crowbar React app renders in it.
- Pane to shell: `window.ipc.postMessage` (string only). Shell to pane: `evaluate_script`.
- Pane bounds follow a GPUI `canvas` prepaint rectangle; resize tracked with no drift after the first `set_bounds`.
- Keyboard goes to the webview after a click; clicking native chrome takes it back.
- Not proven: macOS, IME, drag and drop through the webview, many webviews at once, native Wayland (Linux runs on XWayland, decided).

## 2. Platform rules

- Linux: webviews are X11 children, created and touched only on one GTK thread (`gtk::init` + `gtk::main`, jobs posted through a glib channel). GPUI X11 windows cannot give their handle, so the window id is found by `_NET_WM_PID`. On a Wayland session the shell drops `WAYLAND_DISPLAY` so it runs under XWayland.
- macOS: webviews are created on the UI thread with the window's own handle.
- No Windows.
- A native view paints above GPUI pixels: menus, popovers and drag ghosts over a pane hide that webview briefly. This is a shell-internal visibility change; it sends no lifecycle message.

## 3. Pane host

A pane is `{ paneId, typeId, params, state }` looked up in a registry; the shell never matches on a built-in kind. Built-in React panes and extension panes are the same thing to the host: a URL plus ids.

Host responsibilities: create/destroy/park a webview per pane, place it from the layout rectangle, show/hide, route focus, deliver lifecycle (`mount`, `activate`, `deactivate`, `park`, `destroy`, each with `state`), keep a pane's error inside its frame, show a placeholder for a type that no longer exists.

Webviews are lazy. `keep-alive` panes stay mounted hidden; `parkable` panes destroy their webview and are recreated from `state`; `destroy-on-hide` drops state.

## 4. Daemon transport (replaces what Tauri does today)

Today panes reach the daemon only through Tauri: `crowbar://` fetches proxied to the unix socket (`api_proxy.rs`), and WebSockets relayed by `ws_bridge.rs`. The GPUI shell must provide both; pane code stays unchanged (`web/src/lib/ws/url.ts` already honours `window.__CROWBAR__.api`).

- **HTTP:** wry asynchronous custom protocol `crowbar://<hostId>/v0/...`, proxied to the daemon's unix socket or to a paired remote host (token and TLS pin as in `host_tokens.rs`, `tls_pin.rs`). Same timeouts and connect-retry rules as `api_proxy.rs`.
- **WebSocket:** a webview `WebSocket` cannot dial a unix socket and wry custom protocols cannot upgrade, so the shell runs a loopback relay bound to `127.0.0.1` on a random port. Each pane gets a per-session token and the relay checks it and the Origin before dialling the daemon. The webview uses its native `WebSocket`, so PTY output stays binary with no JS bridging. Chosen over relaying through `ipc` because ipc is string-only and slow for PTY streams.
- **Control frames** (lifecycle, register/open/setTitle/setState, small request/subscribe): JSON text over `ipc`. **Bulk and binary:** the custom protocol, never base64. The sender's identity comes from the per-webview ipc handler, never from a pane-supplied id.

## 5. What moves out of `desktop/src-tauri/src`

Roughly 5,000 lines into Rust crates owned by the shell: `api_proxy`, `ws_bridge`, `hosts`, `host_tokens`, `tls_pin`, `sidecar/*` (daemon supervision, health, watchdog, recovery), `browser/*` (native browser pane), window chrome, context menus, navigation policy, file manager reveal. `desktop/` stays Tauri for mobile only.

## 6. Terminal and editor (native, later)

- Terminal: the daemon sends raw PTY bytes plus ground-state snapshot frames (`api/internal/core/terminal/transport.go`). The shell runs a Rust VT emulator (`alacritty_terminal`) on those frames; GPUI draws the grid. No daemon change.
- Terminal spike result (GPUI 0.2.2, `alacritty_terminal` 0.25.1, software GPU): colors, vim, less, htop, resize, selection, copy and paste all work. Parser speed is 50-70 MB/s; the full window pipeline is 10-18 MB/s on heavy output because every changed row is reshaped, so ship with damage tracking or a row cache. The websocket feed is a tag byte plus bytes (0 = append, 1 = reset then snapshot), one parser per connection. The feed must not forward the local emulator's own query replies, because the daemon already answers them. Missing: IME, mouse reporting, links, wide and emoji fonts, search. Rough remaining work: 2-3 weeks.
- Editor: `gpui-component` editor (Tree-sitter, rope, LSP hooks) is the candidate; diff and search are unverified, so the diff view may stay a webview pane.

## 7. Web host decision: wry

Chosen over offscreen engines. CEF offscreen drew into GPUI with native overlays and ~125-150 MB per extra view, but adds ~300 MB to the app, copies every frame on the CPU, and its GPUI crate is experimental. Servo cannot type into contenteditable, so Plate does not work. WPE has no Rust bindings or macOS support.

Consequences:
- Hidden panes are parked, not kept alive. A small pool of webviews is reused for the visible slots, so memory follows the layout, not the number of open tabs.
- All platform embedding lives in one crate behind `create`, `set_bounds`, `show`, `hide`, `eval`, `destroy`.
- Linux needs the GPUI X11 window handle exposed (vendored patch, then upstream) so the PID scan goes away, plus the GTK thread WebKitGTK requires.
- The daemon may issue per-pane tokens on an authenticated loopback listener instead of a shell-side WebSocket relay; decided with the security review.
- GPUI X11 window handle: a ~15 line patch to `gpui` 0.2.2 (`platform/linux/x11/window.rs`) returns the XCB handle; wry's WebKitGTK path wants Xlib, so the shell re-labels the same window id as Xlib (same id on both connections). The PID scan is deleted. Patch is carried in the vendored gpui and sent upstream.
- Production bundle (software GPU, app idle, MB; PSS counts shared pages once and is closer to real use):

  | Case | RSS | PSS | WebKit web processes |
  |---|---|---|---|
  | app only | ~130 | ~110 | 0 |
  | 1 slot | 350 | 259 | 1 |
  | 4 slots, default | 904 | 456 | 4 |
  | 4 slots, shared WebContext | 775 | 428 | 4 |
  | 4 slots, related views | 410 | 311 | 1 |

  Eight tabs reusing 4 pooled slots cost only ~55 MB more than 4 slots, so memory follows slots, not tabs. Hide frees almost nothing; park frees ~150-190 MB per slot. Restore takes ~0.1-1.2 s depending on mode; `load_url` into an existing slot fires load in 10-60 ms.
- A plain `WebViewBuilder::new()` per webview leaks one network process (~42 MB) each time it is destroyed. The pool must use a shared WebContext or related views (`with_related_view`).
- Related views share one WebKit web process: ~20 MB per extra slot, but no process isolation (a hang in one pane affects all). Open question for the owner: related views for core panes, separate processes for extension panes.
- Unverified: the real app painted only ~295 px of each ~500 px grid cell (possibly app layout, possibly webview sizing); macOS; real GPU.

## 8. Open

- Wire encoding of control frames: proposed after owner review of this plan; sent to the contract agent before use.
- Repo layout of the Rust workspace and the rename of `desktop/` to `mobile/`: owner conversation, deferred.
- macOS verification needs a Mac or a runner.
- Relay hardening choices (token lifetime, Origin rules) are decided with a security review before shipping.

## 9. Settings frames

The shell owns the settings file; panes never persist settings themselves. Two additive frames (sdk@1 pages that do not know them ignore them):

- Shell to pane, `{"kind":"settings","version":1,"values":{...}}`: sent on `mount` right after `theme`, and again whenever a followed setting changes. It is the whole set each time: a key that is absent has its default, so a reset needs no tombstone. At most 128 values; keys are `SettingKey`s. A pane ignores a `version` it does not speak; the version rises only when a key changes meaning, never when one is added.
- Pane to shell, `{"kind":"settings.set","version":1,"key":"showMinimap","value":true}`: asks the shell to store one setting. The shell drops a `version` it does not speak and any key outside the vocabulary (logged), writes the value through its settings backend, and the pane sees it with the next `settings` frame. The pane keeps no copy: its store applies only what the shell pushes.

Vocabulary of version 1 (`PANE_SETTING_KEYS` in `app/crates/protocol`, mirrored in `app/panes/src/features/settings/lib/shell-settings.ts`): editor (`fontFamily`, `fontSize`, `markdownFontSize`, `editorLineHeight`, `tabSize`, `wordWrap`, `lineNumbers`, `renderWhitespace`, `renderIndentGuides`, `semanticHighlighting`, `highlightOccurrences`, `showMinimap`, `autoSave`, `formatOnSave`, `inlineBlame`, `autoCompletion`, `parameterHints`, `maxOpenTabs`), UI typography and `iconTheme` (`uiFontFamily`, `uiFontSize`, `headingFontFamily`), terminal (`terminalFontFamily`, `terminalFontSize`, `terminalLineHeight`, `terminalLetterSpacing`, `terminalScrollback`, `terminalCursorStyle`, `terminalCursorBlink`, `terminalCursorWidth`), browser (`browserHomepage`, `browserSearchEngine`, `browserCustomSearchUrl`, `browserZoom`), file tree and git (`fileTreeIndentSize`, `compactFoldersInFileTree`, `fileTreeDensity`, `showHiddenFilesInFileTree`, `showGitignoredFilesInFileTree`, `hiddenFilePatterns`, `hiddenDirectoryPatterns`, `showGitStatusInFileTree`, `compactGitStatusBadges`, `diffViewMode`), `chatIsDefaultPresentation`. `keybindingUserOverrides` (the user's chord overrides: a pane applies them to the chords it serves itself, such as zoom, save, toggle view, cycle provider and pick). Not in it: `theme` and `themeMode` (they travel as the `theme-id` and `theme-type` tokens of the `theme` frame) and shell-only keys (sidebar, tab list, window, build readout).

Host side: `PaneHost::set_settings` (stored, pushed to mounted panes, re-sent on mount, nothing when unchanged) and `HostEvent::SettingRequested` for a `settings.set`. Shell side: the app watches its settings backend for followed keys, rebuilds the set and calls `set_settings`; `SettingRequested` is written back through `SettingsBackend::set`. Pane side: `followShellSettings` feeds the pane's settings store from the frame and routes `updateSetting` of a vocabulary key to `settings.set` instead of changing the store.

## 10. Unsaved work frames

A pane that can hold work closing would lose reports it, and the shell asks before it closes the pane. Two additive frames (sdk@1 pages that do not know them ignore them):

- Pane to shell, `{"kind":"pane.setDirty","dirty":true}`: the whole truth each time, sent whenever the flag changes (`PaneClient.setDirty`; `HostEvent::DirtyChanged`). The shell keeps the set of dirty panes and forgets a pane that leaves the layout.
- Shell to pane, `{"kind":"pane.save"}`: the user chose Save in the Unsaved Changes dialog (`PaneClient.onSave`). The pane saves and reports `dirty:false`; a failed save leaves it dirty, so the pane stays open and the close keeps waiting.

Guard points: closing a tab or pane from the strips (`ClosePane`, `CloseTab`, so bulk and group closes too), `mod+w` and closing the window. The dialog offers Cancel, Discard and Save. Tab-cap eviction never takes a dirty pane (`app/desktop/app/src/tab_cap.rs`).

## macOS web view snapshot (open, gate item)

`Platform::snapshot` returns no pixels on macOS, so overlays hide the pane instead of freezing it. Implement with `WKWebView.takeSnapshot` through `objc2-web-kit` and prove it on the macOS CI runner; crate versions and feature names must be taken from that runner's resolver, not guessed on Linux. Data store isolation per extension needs macOS 14+; older systems need a documented fallback.
