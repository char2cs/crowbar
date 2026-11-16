# Rabbyte rules audit, 2026-11-16 (branch migration/rust-shell)

Audit only: no source edited, no build run. The tree was being edited by other agents while this ran, so the
gate output below is a snapshot (12:23 UTC); two earlier runs 12 minutes apart differed by about 40 findings.
Re-run the gates before and after fixing. Rules referenced by manifest number (docs/rabbyte-manifest.md);
Go-only rules 18-19 are n/a, 14-17 and 20-23 are covered by cargo/SwiftPM and the scripts.

Method: every CI gate run as CI runs it; heuristic scans (python/grep, in the audit scratchpad) over 937
non-test Rust sources (app/crates/*, app/desktop/crates/*, app/desktop/app, app/mobile/ios-plugin,
app/mobile/src-tauri), 115 Swift sources + 30 Swift tests, 66 new TS sources (+ 48 tests) of app/panes; manual
reading of about 45 Rust, 25 Swift and 30 TS files chosen across all crates (largest, script-flagged, and
risk-pattern hits); mutation tests of the scripts with deliberately bad code outside the repo.

## 1. Mechanical gates

| Gate (as in .github/workflows/rust.yml) | Result |
|---|---|
| `check-standards.sh --self-test` (rust, swift, ts) | all fixtures pass (0 FAIL) |
| `check-rust-rules.sh` (CI default roots app/crates, app/desktop) | FAIL, see 1.1 |
| `check-rust-rules.sh --mobile` (standards job, adds ios-plugin) | FAIL, adds `src/frontend.rs:15 [global]` |
| `check-rust-rules.sh app/mobile/src-tauri` (not in any CI root) | clean, but never run by CI |
| `check-swift-rules.sh app/mobile/ios` | clean (146 files); `--full` cannot run here (swiftlint, swift-format not installed) |
| `check-ts-rules.sh` (changed files vs develop, as CI) | FAIL, 9 findings, see 1.3 |
| `check-ts-rules.sh --all` | 19,047 findings, all but 3 in legacy files carried over from `web/` (see 1.3) |
| `check-cycles.py` (via the rust/ts scripts) | TS: 3 cycles, all in legacy files (only seen with `--all`); Rust: see 1.1 |
| `check-test-names.sh` | ok (2 baselined) |
| cargo fmt/clippy/deny/machete/test, swiftlint, eslint, knip, tsc | NOT RUN (builder busy, tools missing); every "clippy covers it" claim below is unverified |

### 1.1 Rust script findings (snapshot 12:23, `--mobile`)

```
browser_nav/src/page_update.rs:1: [multi-type] 3 top-level types in one file: PageUpdate, PickMessage, ...
browser_nav/src/page_update.rs:82: [depth]
browser_nav/src/pointer_text.rs:1: [table-driven] 4 #[test] fns and no CASES table
browser_nav/src/pointer_text.rs:86: [else]
transport/src/local.rs:21: [doc-private]
transport/src/socket_trust.rs:1: [table-driven] 3 #[test] fns
app/src/first_run.rs:10,11,12: [comment] inline comment
app/src/first_run.rs:1: [type-name] type Landing must live in landing.rs
app/src/main.rs:149: [depth]
app/src/pending_close.rs:111: [depth]
app/src/shell_view/closing.rs:139: [depth]
app/src/shell_view/first_run.rs:1: [test-missing]
app/src/shell_view/picking.rs:1: [type-name] type PickEffect must live in pick_effect.rs
app/src/shell_view/picking.rs:36,40,43,85: [depth]
app/src/shell_view/remote_folder.rs:57: [depth]
app/src/window_title.rs:51,61: [bool-param]
crates/browser_chrome/src/browser_bar.rs:59: [bool-param]   (set_picking(picking: bool))
crates/browser_chrome/src/browser_bar.rs:94: [else]
crates/daemon_supervisor/src/stale_daemon.rs:45: [real-infra]
crates/oobe_view/src/backdrop.rs:1, check_source.rs:1, look.rs:1, pill_button.rs:1: [test-missing]
crates/oobe_view/src/check_status.rs:4: [doc-private]; :14 [comment]; :101 [bool-param]
crates/oobe_view/src/oobe_view.rs:184: [fn-length] 42 lines; :394 [depth]
crates/oobe_view/src/pill_button.rs:14: [doc-private]; :37 [bool-param]
crates/oobe_view/src/test_support/fake_check.rs:15: [bool-param]
crates/pane_host/src/browser_slot.rs:41: [depth]
crates/pane_host/src/inbox_gate.rs:14,15: [comment]
crates/pane_host/src/pane_host_frames.rs:240: [fn-length] 43 lines
crates/settings_ui/src/catalog/keybindings.rs:22: [doc-private]; :27 [else]
crates/settings_ui/src/keymap.rs:204: [fn-length] 41 lines
crates/settings_ui/src/record_flag.rs:12: [doc-private]; :1 [multi-type] RecordFlag, FlagLook
crates/settings_ui/src/record_view.rs:39: [fn-length] 43; :110 [depth]; :199 [bool-param]
crates/shell_data/src/icon_fetch.rs:1: [table-driven] 3 #[test] fns
crates/sidebar/src/file_drop.rs:42: [depth]
crates/sidebar/src/file_tree_row.rs:51: [fn-length] 41 lines
crates/shell_keys/Cargo.toml:1: [entry-file] crate entry lives in src/shell_keys.rs
src/frontend.rs:15 (ios-plugin): [global] static FRONTEND: OnceLock
```

Counts: depth 14, bool-param 6, test-missing 5, fn-length 5, doc-private 5, table-driven 3, else 3,
comment 6, type-name 2, multi-type 2, real-infra 1, entry-file 1, global 1. Earlier in the pass the same run also
showed `[cycle] module cycle: close_step <-> pending_close` (app/src/close_step.rs:2 imports
`crate::pending_close`, pending_close.rs:4 imports `crate::close_step`); it was gone at 12:23, re-check.

### 1.2 Swift: clean (script). Rust `--full`/swiftlint not runnable.

### 1.3 TypeScript

CI form (changed files only), snapshot 12:23:

```
src/__tests__/entries/buffer-pane-host.test.tsx:58 [depth]
src/__tests__/lib/ws/shell-socket.test.ts:1 [table-driven] 4 it() blocks and no it.each
src/entries/buffer-pane-host.tsx:28 [fn-length] 55 lines
src/entries/follow-shell-appearance.ts:12 [comment]
src/lib/ws/shell-socket.ts:8 [doc-private]; :14 [magic-number] 200
src/sdk/pane-client.ts:75 [bool-param] setDirty(dirty: boolean); :127,133,134,138 [depth] (call())
```

`--all` totals by rule (19,047, of which 19,044 are in 1,738 legacy files that also exist under `web/` on
develop and are skipped by CI): comment 10,544; depth 3,208; doc-missing 1,420; doc-private 565;
table-driven 550; fn-length 406; test-missing 395; magic-number 362; else 266; long-condition 236;
multi-concept 207; type-name 144; bool-param 122; test-orphan 116; global 102; non-null 84; test-style 68;
module-name 65; layer 49; param-count 47; hand-loop 28; any 17; suppress 11; barrel 8; named-return 6;
sleep 5; file-name 5; new-stateless 3; cycle 3; console 3; todo 1; test-scenario 1.
The owner's "no merge if a rule is broken" is currently satisfied for TS only because legacy files are
exempt by design (`changed_files` in check-ts-rules.sh). That is a decision to confirm, not a pass.

## 2. Rule by rule

PASS = no violation found by script or sampling. FAIL = at least one. WARN = judgement call for the owner.

### Rule 1 parameters one per line, max 4: FAIL (workarounds)
- 267 parameters are packed into tuple patterns to stay under the 4-parameter limit and past `param-count` and
  `bool-param`: `(page, index, block): (&Page, usize, &Block)`. Worst files: shell_view/file_actions.rs (9),
  shell_data/plan_create.rs (8), settings_ui/visibility.rs (6), settings_ui/record_view.rs (6),
  terminal_session/terminal_sessions.rs (5), sidebar/file_tree_row.rs (5), ui/toast_queue.rs (4),
  sidebar/tree_rows.rs (4), sidebar/menu_list.rs (4), sidebar/file_menu.rs (4), shell_data/shell_data.rs (4),
  shell_data/settings_core.rs (4), shell_data/env_template_file.rs (4), settings_ui/shortcut_row.rs, list_view.rs,
  body.rs (4 each), layout_view/strip_input.rs (4). Fix: a named struct per call.
- Crate-level `#![allow(clippy::too_many_arguments, reason = "the four-parameter limit of the manifest does not
  count self")]` in terminal/src/lib.rs:11, terminal_session/src/lib.rs:6, terminal_view/src/lib.rs (and per-fn
  `#[expect]` in shell_view/shell_commands.rs:180,256,296, shell_view/file_actions.rs:203). This loosens a lint
  (CLAUDE.md: never loosen). Remove, introduce parameter structs.
- app/desktop/app/src/settings_modal.rs:18 `open_settings` has 5 parameters and is not reported (script blind
  spot, section 4).

### Rule 2 depth max 2: FAIL, 14 script findings (list in 1.1; most per file: shell_view/picking.rs 4, oobe_view)
### Rule 3 guards, no else: FAIL
- else: browser_nav/pointer_text.rs:86, browser_chrome/browser_bar.rs:94, settings_ui/catalog/keybindings.rs:27.
- Workaround: `match <bool> { true => .., false => .. }` 22 arms (terminal_view/terminal_view_mouse.rs 5,
  terminal/terminal.rs 3, terminal_session/stream.rs 2, terminal/viewport.rs 2, terminal/row_builder.rs 2,
  api_proxy/proxy_answer.rs, terminal_view_paint.rs, search_bar.rs), legalised by
  `#![allow(clippy::match_bool, ...)]` in terminal/src/lib.rs:4, terminal_session/src/lib.rs:3,
  terminal_view/src/lib.rs. Not an else, but it is the same branch; use guard + early return.
### Rule 4 named returns: PASS by script. 11 hand-built accumulators (for + push) the script cannot see:
  terminal/link_finder.rs:89, settings_ui/section_view.rs:28, list_view.rs:81, shortcuts_view.rs:24, rail.rs:19,
  command_palette/fuzzy_match.rs:77, palette_row.rs:15, shell_data/file_upload.rs:45, new_id.rs:28,
  sidebar/removal_tray.rs:63 (also rule 6).
### Rule 5 named conditions: PASS (script flags only 3+ operands; 2-operand conditions pass)
### Rule 6 stdlib over loops: FAIL (minor): the 11 sites above; 150 `for/while/loop` in 106 Rust files, most are
  fine side-effect loops. Swift/TS script clean.
### Rule 7 comments: FAIL
- `SPEC:` is used as a prefix to launder ordinary rationale: 25 comments in Rust/TS/Swift cite no spec, RFC or
  URL (the script accepts the prefix alone). Reword into the code or delete: browser_nav/browser_nav.rs:5,
  app/focus_neighbour.rs:81, core_type.rs:6-7, pane_spec.rs:141, diagnostics_export.rs:12,
  shell_view/shell_commands.rs:18,98, shell_keys/keystroke.rs:20,48, command_table.rs:20,
  pane_host/browser_slot.rs:18,27,35, slot_factory.rs:83, TS entries/buffer-pane-host.tsx:40,
  entries/chat-pane-mount.tsx:38, lib/ws/channel-handlers.ts:23,72, lib/ws/channel-subscriptions.ts:12,30,
  features/hosts/lib/ws-host-ports.ts:7, features/settings/lib/shell-settings.ts:6,
  Swift CrowbarIcon.swift:1, OKLabMatrix.swift:3 (the last two are legitimate).
- Script: first_run.rs:10-12, check_status.rs:14, inbox_gate.rs:14-15, follow-shell-appearance.ts:12;
  doc on private items: transport/local.rs:21, check_status.rs:4, pill_button.rs:14, record_flag.rs:12,
  keybindings.rs:22, TS shell-socket.ts:8.
- 24 `PLATFORM:` Rust comments and 19 Swift ones were skimmed; the Swift ones are genuine platform notes.
- Interface members in TS (`unwatchIfNoneParked` in lib/ws/parked-hosts.ts) have no doc comment, the script
  checks only exported top-level symbols.
### Rule 8 one type per file: FAIL
- multi-type: browser_nav/page_update.rs (PageUpdate + private PickMessage, PickBody), settings_ui/record_flag.rs
  (RecordFlag, FlagLook); pane_host/inbox_gate.rs was flagged earlier (Kind, InboxGate) and has been fixed in the
  12:23 snapshot.
- type-name: app/src/first_run.rs (Landing -> landing.rs), app/src/shell_view/picking.rs (PickEffect -> pick_effect.rs).
- One type spread over many files (reads as arbitrary splitting, rules 8/31/45): PaneHost over pane_host.rs +
  pane_host_frames/freeze/slots/refusals.rs; TerminalView over terminal_view_mouse/paint/ime/session.rs; TextArea
  over text_area_*.rs (9 files); TextInput over text_input_*.rs; ShellView over shell_view/*.rs (15 files).
### Rule 9 error beside struct: PASS (script)
### Rule 10 entry file named after crate: FAIL, crates/shell_keys (Cargo.toml:1; lib exports a free function but
  no src/shell_keys.rs).
### Rules 11, 12 stateless type / new: PASS (script; verified by mutation)
### Rule 13 public API exposes traits: WARN. 254 `pub struct` vs 35 `pub trait`; 365 `pub` fields in public structs
  (daemon_supervisor/timings.rs 16, sidebar/row.rs 14, transport/relay_config.rs 11, terminal_view/terminal_settings.rs 8,
  terminal/snapshot.rs 8, sidebar/merge_box.rs 8, sidebar/file_tree_view.rs 8, browser_nav/pointer_item.rs 8, ...).
  rust-style allows concrete types with private fields (C-STRUCT-PRIVATE); pub-field records contradict that.
### Rule 14 root exposes only API: WARN. lib.rs re-export counts: ui 81 `pub use` + 9 `pub mod`, sidebar 62,
  settings_ui 53, terminal 43, protocol 43, shell_data 37, transport 26, pane_host 26. ui/src/lib.rs:42-149 `pub mod`
  token modules.
### Rules 15-17 internal / layering: PASS by cargo-layer script; not verified with cargo (no build).
### Rule 20 no cycles: FAIL (TS legacy): lib/api/fetch.ts <-> lib/crowbar-bridge.ts <-> lib/ws/url.ts;
  lib/api.ts <-> lib/project-url.ts; features/panes/stores/slices/buffer-slice.ts <-> pane-actions/editor-tab-actions.ts.
  Rust: see 1.1 note (close_step <-> pending_close, transient).
### Rules 21-23 names / stutter: PASS (sampled). Duplicate trait name `Clock` in layout_view/src/clock.rs and
  tab_bar/src/clock.rs (also rule 36).
### Rule 24 no globals/init: FAIL
- mobile/ios-plugin/src/frontend.rs:15 `static FRONTEND: OnceLock<Frontend>` (script), ffi.rs:77 `static INSTALL: Once`.
- TS features/settings/lib/shell-settings.ts: module-level `useShellRoute = create(...)` holds a mutable `send`
  (the script only sees `let`/`var`).
### Rule 25 context/cancellation: n/a for Rust; sampled TS (`setTimeout` in lib/ws/channel-handlers.ts:80,
  sdk/pane-client.ts:127) has no cancel on dispose.
### Rule 26 errors never ignored, wrapped: FAIL
- `Result<_, String>` 91 sites (shell_data/plan_create.rs 12, mobile api_proxy/pairing.rs 8, plan_edit.rs 6,
  plan_merge.rs 5, ws_bridge.rs 4, planner.rs 4): strings drop the source error.
- `.map_err(|_| ...)` dropping the cause: browser_nav/browser_url.rs:21, transport/daemon_wire.rs:28,43,48,55,
  transport/gate.rs:42,94,104,127, transport/remote.rs:20, mobile ws_bridge.rs:76, api_proxy/remote.rs:163,
  desktop/app/sidebar_requests.rs:20, shell_diagnostics/shell_diagnostics.rs:36. (Timeout `Elapsed` and
  closed-channel cases such as supervisor.rs:66-67, linux_platform.rs:38 carry no cause and are fine.)
- Swallowed `Err(_)`: transport/local.rs:61 `Err(_) => {}`, daemon_link.rs:138, daemon_supervisor/health.rs:131,
  process_cpu.rs:32, sidebar/act.rs:138 `Err(_) => false`, app/shell_view/browsing_data.rs:13,
  shell_data/envelope.rs:39, pane_host/pane_host_slots.rs:41.
- Discarded `.ok();` on sends/updates: transport/probe.rs:54, shell_transport.rs:193,211, mobile
  ws_bridge/remote.rs:121,131, shell_view/file_actions.rs:125,243, shell_view/data_events.rs:29,157,
  daemon_supervisor/bus.rs:28, startup_view/startup_view.rs:171, ui/toast_queue.rs:234, shell_data/hub.rs:66,
  sidebar/space_icon_editor.rs:18, create_space_panel.rs:77, `let _ = assets();` ui/src/ui.rs:47.
- Swift `try?` SettingsModel.swift:13, ChromeOverlays.swift:165 (decode failure silently ignored).
- 98 other `.ok()` conversions and 56 `unwrap_or_default` were not individually triaged; manifest M26 forbids
  `unwrap_or_default` that hides failure.
### Rule 27 no panic/unwrap/expect on runtime paths: PASS for Rust library code (7 unwraps, all in
  desktop `src/bin/*/scenario.rs` demo binaries). `unreachable!` is not forbidden by the script or the clippy
  table (1 use). TS: `JSON.parse(text) as ShellFrame` in sdk/pane-client.ts:receive is an unchecked cast of
  external input.
### Rule 28 small traits: FAIL. Traits with more than 3 methods: sidebar/sidebar_data.rs SidebarData 11,
  ui/text_area_surface.rs TextAreaSurface 10, webview/platform.rs Platform 8, ui/text_surface.rs TextSurface 7,
  pairing_view/pairing_source.rs PairingSource 6, settings_ui/settings_backend.rs SettingsBackend 5,
  remote_folder_picker/folder_source.rs FolderSource 5.
### Rule 29 names: WARN. Abbreviations: `resp`/`msg` in mobile/src-tauri (api_proxy/proxy_answer.rs, remote.rs,
  pairing.rs 23 sites); TS `ch` (31), `cb` (9) in lib/ws/*.ts; tuple-destructured parameter names
  (`(quote, escaped)`) hide intent at the signature.
### Rule 30 one thing, small: FAIL, 5 script fn-length (pane_host_frames.rs:240, keymap.rs:204, record_view.rs:39,
  oobe_view.rs:184, file_tree_row.rs:51, TS buffer-pane-host.tsx:28). Candidates over the threshold of
  "one thing": ts lib/ws/channel-handlers.ts:handleOpen (resets delay, flushes outbox, probes daemon, reports,
  resolves waiters, notifies), Swift ChromeOverlays.swift:wire.
### Rule 31 single responsibility: FAIL (candidates). Files with the most functions: settings_ui/settings_state.rs 40,
  terminal/terminal.rs 39, daemon_supervisor/daemon_supervisor.rs 36, desktop/app/shell_state.rs 36,
  shell_data/settings_core.rs 35, model/tab.rs 34, terminal_view/terminal_view_mouse.rs 33,
  model/split_node.rs 30, pane_host/pane_host.rs 28; 72 files have more than 12 functions. TS sdk/pane_client.ts
  `PaneClient` (register, lifecycle, themes, settings, state, requests, subscriptions, timeouts).
### Rule 32 no magic numbers/strings: FAIL
- Script: TS shell-socket.ts:14 (200).
- Blind to hex and underscore literals: 550 sites in 141 files (excluding generated palettes: tab_bar/tab_bar_render.rs 9,
  ui/keyframes.rs 9, sidebar/sidebar_frame.rs 9, sidebar/avatar_tint.rs 8, ui/dialog.rs 6, ui/thumb.rs 6,
  model/split_node.rs 5).
- Magic strings are unchecked: Swift SettingsModel.swift:7 `"system"`, Dialogs.swift:66 `"Cancel"`, global names
  `"__crowbarSettingsTab"` and others in ChromeOverlays.swift (28 literals), RowMenu.swift (13); Swift
  NavigationController.swift:103,105,112 `0.4`, `0.6`, `3` in `case ... where` (script skips `case` lines).
### Rule 33 no bool params: FAIL
- Script: app/window_title.rs:51,61, browser_chrome/browser_bar.rs:59 `set_picking(picking: bool)`,
  oobe_view/check_status.rs:101, pill_button.rs:37, test_support/fake_check.rs:15, settings_ui/record_view.rs:199
  `dot_box(on: bool, ..)`, TS sdk/pane-client.ts:75 `setDirty(dirty: boolean)`.
- Hidden in tuples (script blind): shell_view/file_tree_flag.rs:18 `(key, on)`, settings_ui/visibility.rs:89,101,114,129
  `(words, whole)`, shell_data/file_tree_reads.rs:16, env_template.rs:29, merge_inputs.rs:73 `(u32, bool, bool)`,
  entity_store.rs:87, ui/text_buffer.rs:127 `(clicks, extend)`, ui/text_row.rs:42, sidebar/file_tree_row.rs:91,249,
  menu_list.rs:78,80, menu_entry.rs:43; plain bools missed because of multi-line params in `pub(crate)` free functions
  are caught only by the bool regex; shell_view/shell_commands.rs:99,203 `on_terminal: bool`; dirty_panes.rs:13
  `set(pane, dirty: bool)`.
- TS call sites `setOpen(true/false)` (entries/console-pane-mount.tsx:34,51), JSX `showing`/`editorViewHidden={false}`.
### Rule 34 command-query separation: FAIL. `&mut self` methods that also return information (30 flagged by
  heuristic; the genuine ones): model/layout_history.rs:114 `drop_oldest -> bool`, :122 `undo -> bool`, :135 `redo -> bool`;
  model/split_node.rs:117 `split_leaf -> bool`; transport/token_book.rs:43 `authorize -> Option<RelayScope>`, :71
  `retain_hosts -> bool`; app/shell_state.rs:146 `try_apply -> bool`, :197 `reopen_closed -> bool`;
  shell_data/folder_held.rs:29,40, pairing_held.rs:18, state.rs:163 `drop_local -> bool`; terminal/segment.rs:22
  `trim_blanks -> bool`; ui/text_area_mouse.rs:73,83; sidebar/removals.rs:31 `request -> bool`, drag.rs:110
  `arm_lift -> bool`, act.rs:120 `send_checked -> bool`; daemon_supervisor/failure_tracker.rs:20 `observe -> bool`;
  settings_ui/visibility.rs:85-126 `add_* -> bool`. TS: features/settings/lib/shell-settings.ts:
  `writeSettingToShell` sends and returns a boolean.
### Rule 35 no hidden side effects: WARN (sample). Swift ChromeOverlays.swift `wire` assigns six collaborators
  and mounts navigation; TS lib/ws/open-channel.ts `park()` creates a channel, registers it in the context map,
  reports state and starts a watch.
### Rule 36 duplication at the third repetition: FAIL
- transport/pinned_verifier.rs, transport/unpinned_probe_verifier.rs, mobile/src-tauri/tls_pin/pinned_verifier.rs,
  mobile/src-tauri/api_proxy/probe/unpinned_probe_verifier.rs: the same rustls verifier in four files, two crates.
- Swift ChromePlugin.swift: the `DispatchQueue.main.async { if #available(iOS 26.0, *) { ...; invoke.resolve() } }`
  wrapper 13 times (lines 20-117).
- The 46 pane setting keys exist twice: protocol/src/pane_setting_keys.rs and TS
  features/settings/lib/shell-settings.ts `SHELL_KEYS` (also a second copy of state per CLAUDE.md).
- `on_input` in terminal_view/search_bar.rs, command_palette/palette_panel.rs, branch_import_view/branch_import_view.rs,
  pairing_view/pairing_view.rs (4x); two `Clock` traits; Swift `.toolbarBackgroundVisibility` sheet chrome in
  HostPickerSheet, ModelPickerSheet, SettingsSheet.
### Rule 37 dead code: PASS by sampling (no commented-out code found in 937 + 115 + 66 files; no `allow(dead_code)`);
  compiler dead-code, machete and knip NOT run.
### Rule 38 dependency injection: FAIL
- Rust production code reaching for the clock directly: `Instant::now()` in ui (tabs.rs:96, toggle_state.rs:35,
  slide_state.rs:35, tooltip.rs:134, hover.rs:40, text_area_element.rs:80, text_input_element.rs:76), sidebar
  (peek.rs:29,36,43, hover_glide.rs:45,55, act.rs:193,204,216, tree_rows.rs:45, chats.rs:98, drag_line.rs:26),
  daemon_supervisor/watchdog.rs:30,54,86, daemon_supervisor.rs:373, transport/relay.rs:112, relay_state.rs:42,
  desktop/app/readout_state.rs:19, shell_view/readout.rs:91, terminal_view.rs:105.
- Direct filesystem / process / env in about 30 files with no port: transport/hosts_document.rs, host_registry.rs,
  desktop/app/layout_store.rs, diagnostics_export.rs:36, daemon_link.rs:205, startup_command.rs:39,
  window_state/window_store.rs:27, shell_data/host_notes.rs:29, setting_file.rs:34, icon_fetch.rs:101-103,
  plan_icon.rs:34, sidebar/preferences.rs:70, pane_host/dir_bundle.rs:45, shell_diagnostics/bundle.rs, crash_report.rs.
- Swift: AppScheme.swift:49 `URLSession.shared`; `var send: (ChromeIntent) -> Void = { _ in }` collaborators assigned
  after construction instead of passed to init: ChromeModel.swift:15-16, SettingsModel.swift:8-9, TabsModel.swift:5,
  HostPickerModel.swift:5, ModelPickerModel.swift:14, NavigationController.swift:24-25, DrawerSwipe.swift:16,
  PageChannel.swift:11, StripCatcher.swift:5, KeyboardFit.swift:7; wiring in ChromeOverlays.swift:wire.
- TS: `new WebSocket` in lib/ws/create-transport.ts:28, shell-socket.ts:10 (behind ports, acceptable);
  `document`/`window` in entries/pane-boot.ts:20.
### Rule 39 table-driven tests: FAIL, script: browser_nav/pointer_text.rs, transport/socket_trust.rs,
  shell_data/icon_fetch.rs, TS __tests__/lib/ws/shell-socket.test.ts. Blind: the script accepts any file in which
  the word `cases` or `CASES` appears, including comments.
### Rule 40 test the interface: not scanned; inline tests are `use super::*` by design.
### Rule 41 no real infrastructure: FAIL. Script: daemon_supervisor/stale_daemon.rs:45. Not seen by script: real
  loopback TCP and Unix sockets and temp dirs in unit tests: terminal_session/terminal_sessions.rs:385
  (`TcpListener::bind`), mobile api_proxy/probe.rs:173, transport/local.rs:137-152, local_get.rs:31-57,
  daemon_supervisor/watchdog.rs:199-221 (also `tokio::time::sleep` 3s/60s without paused clock at :221,224),
  daemon_supervisor/{log_writer,daemon_log,output_pump,spawn_daemon,locate_binary}.rs (tempfile), transport/hosts_document.rs,
  host_registry.rs, app/layout_store.rs, shell_diagnostics/bundle.rs, transport/tests/transport.rs, relay.rs.
  Production timers to review against CLAUDE.md "no timer/poll/retry fix": daemon_supervisor/health.rs:101
  health poll, transport/transport.rs:150 connect retry, shell_data/host_stream.rs:56 backoff,
  terminal_session/terminal_sessions.rs:227 reconnect, TS lib/ws/channel-handlers.ts:80 reconnect.
### Rule 42 1:1 tests: FAIL
- Rust: test-missing app/src/shell_view/first_run.rs, oobe_view/{backdrop,check_source,look,pill_button}.rs (exemption
  list is empty).
- Swift: 115 sources, 30 test files, 16 exemptions. The script only demands a test when the file contains `func`;
  files with only computed properties, `init`, `body` or state machines never need one.
- TS: ok.
### Rule 43 no free tests: PASS (script). `app/crates/model/tests/model.rs` 500 lines and `transport/tests/relay.rs`
  428 are public-API integration tests, allowed.
### Rule 44 size: PASS by script. 3 files exceed 500 raw non-test lines and pass only because the counter drops
  blank and `///` lines: pane_host/src/pane_host.rs (505), shell_data/src/settings_core.rs (504),
  terminal_view/src/terminal_view_mouse.rs (503). Treat as rule 45 candidates.
### Rule 45 overflow means wrong abstraction: see 8/31; the `*_mouse/_paint/_ime`, `pane_host_*`, `text_area_*`
  families are the same type cut into files.

Counts: FAIL on rules 1, 2, 3, 6, 7, 8, 10, 20, 24, 26, 28, 30, 31, 32, 33, 34, 36, 38, 39, 41, 42; WARN on
13, 14, 29, 35; PASS on 4, 5, 9, 11, 12, 15-17, 21-23, 27, 37, 40, 43-45 (with the caveats above).

## 3. Does the tooling enforce what docs/standards-coverage.md says?

Mutation tests (scripts run on deliberately bad code in the audit scratchpad). Confirmed working:
param-layout, param-count, depth, else, long-condition, comment, bool-param (fn signatures), magic-number
(plain decimals), stateless-type, new-stateless, panic (unwrap), multi-type, test-missing, entry-file, table-driven,
real-infra (patterns listed in script), Swift try!, global, hand-loop, TS any/non-null/console/suppress/global/
file-name/doc-missing/multi-concept. All fixture self-tests pass.

Claimed but not enforced, or enforced narrower than the table says:
1. Rule 1 (Rust): `param-count` and `param-layout` are skipped for `pub(crate)` and `pub(super)` functions. The
   awk takes the first `(` of the line, which is the one in `pub(crate)`, so only the `bool-param` regex still sees the
   signature. Verified: a `pub(crate) fn` with 5 parameters and one with two parameters on one line report nothing, the
   same code as `pub fn` reports. Real hit: app/src/settings_modal.rs:18. (brace-rules.awk, `decl_start`/`check_params`.)
2. Rules 1 and 33: tuple-packed parameters bypass `param-count`, `bool-param` and `param-layout` (267 sites). The
   bool regex needs `: bool` followed by `,` or `)`, so `(words, whole): (&[String], bool)` never matches.
3. Rule 33: closures (`|verbose: bool|`) and Swift `(Bool) -> Void` parameter types are not seen; `impl Trait for`
   methods are exempt by design.
4. Rule 32: hex (`0xFF`), underscore (`1_000_000`) and expressions like `10 * 100` pass; any line starting with
   `case`, or containing `assert`, `const`, `static`, `#[` is skipped entirely. Magic strings: no check, although
   the table says "sample" (the sampling procedure does not appear to run; nothing under docs/review lists it).
5. Rule 4: `named-return` only matches accumulators named result, ret, res, out, output, acc, accum, retval; `let mut total`
   or `rows` returned at the end passes.
6. Rule 6 (Rust): coverage table says clippy only; the script has no `hand-loop`. Fine, but unverified without a build.
7. Rule 7: `SPEC:`/`PLATFORM:`/`SAFETY:`/`DEPRECATED:` prefixes are accepted whatever follows (25 SPEC comments cite
   nothing). TS `doc-missing` covers exported top-level symbols only, not interface members or class methods.
8. Rule 20 (Rust): `check-cycles.py` reads only `src/*.rs` (no subdirectories, so `shell_view/*`, `settings_state/*`
   are never graphed) and only `crate::x` paths; `super::` and `self::` edges are invisible. Verified: a two-module cycle
   written with `super::` reports nothing. TS: cycles are reported only when they include a changed file, so the 3 legacy
   cycles are silent in CI.
9. Rule 27: `unreachable!` and `assert!` are not treated as panics by the script; clippy `panic` does not cover
   `unreachable`. `print!`-family is clippy only.
10. Rule 39: any occurrence of the text `cases` or `CASES` anywhere in the file (even a comment) satisfies
    `table-driven`; the threshold is more than 2 tests, so two-test modules never need a table.
11. Rule 41: `real-infra` checks five call patterns; loopback sockets (`TcpListener::bind`, `UnixListener`),
    `tempfile`, `std::fs` and `tokio::time::sleep` in tests are not flagged (about 90 sites).
12. Rule 42 (Swift): `test-missing` only fires for files that contain `func`; Rust likewise needs a `fn`. Constructor
    or property-only logic needs no test.
13. Rule 16/17 (TS): the `layer` script matches `from '@/components/...'` only. A relative import such as
    `../components/thing` from `src/lib` passes (verified), as do double-quoted and `import()` forms.
14. Rule 24 (TS): only `let`/`var` module bindings; module-level zustand stores and `new Map()` singletons pass.
15. Rule 44 (Rust): counts without doc comments and blanks, which the manifest does not exempt ("lines of code" is
    defensible, but the 500 ceiling is reached on raw size in 3 files).
16. Coverage gaps in CI scope, not in the awk: `app/mobile/src-tauri` is not in the default roots of
    check-rust-rules.sh (clean today, unguarded); the TS checks run only in the `standards` job, which runs on
    non-draft pull requests only (rust.yml, `if: pull_request && draft == false`), never on pushes or draft PRs;
    the Rust script's `--mobile` runs only there too (the `rust-lint` job runs it without `--mobile`); the `--full`
    columns (swiftlint, swift-format, clippy, cargo-deny, machete) require tools absent here, so the whole
    "clippy covers it" column was not verified by this audit.
17. standards-coverage.md says the sample procedure checks changed files each cycle, but docs/review contains no
    sampled-review output other than the production review; rules 10-11, 13, 23, 25, 28, 29, 31, 32 (strings),
    34-36, 38, 40, 45 therefore have no running check. Findings above for those rules are the first.

## 4. Fix list, per file (work top to bottom; items marked (S) are script findings)

Rust, scripts:
- app/crates/browser_nav/src/page_update.rs: move PickMessage and PickBody to pick_message.rs/pick_body.rs; fix depth at :82 (S).
- app/crates/browser_nav/src/pointer_text.rs: CASES table for the 4 tests; remove else at :86 (S).
- app/crates/transport/src/socket_trust.rs, shell_data/src/icon_fetch.rs: CASES table (S).
- app/crates/transport/src/local.rs:21 remove doc on private item (S); :61 `Err(_) => {}` log or propagate (rule 26).
- app/desktop/app/src/first_run.rs: rename file to landing.rs or move type; delete comments :10-12 (S).
- app/desktop/app/src/main.rs:149, pending_close.rs:111, shell_view/closing.rs:139, picking.rs:36,40,43,85,
  remote_folder.rs:57: extract functions (S, depth).
- app/desktop/app/src/shell_view/first_run.rs: add inline tests (S).
- app/desktop/app/src/shell_view/picking.rs: move PickEffect to pick_effect.rs (S).
- app/desktop/app/src/window_title.rs:51,61: replace bool params with enums (S).
- app/desktop/crates/browser_chrome/src/browser_bar.rs:59,94: `set_picking(bool)` -> `begin_picking`/`end_picking`; remove else (S).
- app/desktop/crates/daemon_supervisor/src/stale_daemon.rs:45: replace real call with a fake (S); watchdog.rs:221,224: paused clock.
- app/desktop/crates/oobe_view/src/{backdrop,check_source,look,pill_button}.rs: inline tests (S);
  check_status.rs:4,14,101, pill_button.rs:14,37, test_support/fake_check.rs:15, oobe_view.rs:184,394 (S).
- app/desktop/crates/pane_host/src/browser_slot.rs:41, pane_host_frames.rs:240, inbox_gate.rs:14,15 (S).
- app/desktop/crates/settings_ui/src/catalog/keybindings.rs:22,27, keymap.rs:204, record_flag.rs:1,12 (move FlagLook),
  record_view.rs:39,110,199 (S).
- app/desktop/crates/sidebar/src/file_drop.rs:42, file_tree_row.rs:51 (S).
- app/desktop/crates/shell_keys: add src/shell_keys.rs holding the entry function (S).
- app/mobile/ios-plugin/src/frontend.rs:15: remove the OnceLock static, pass the frontend in (S); ffi.rs:77 same.

Rust, not caught by scripts (ranked by count):
1. Replace the 267 tuple-packed parameters with named structs (worst files listed under rule 1) and delete the three
   crate-level `too_many_arguments`/`match_bool` allows in terminal, terminal_session, terminal_view lib.rs.
2. Rule 26: introduce typed errors for `Result<_, String>` (plan_create.rs, plan_edit.rs, plan_merge.rs, planner.rs,
   pairing.rs, ws_bridge.rs); keep the cause in the 14 `map_err(|_| ..)` sites listed; handle the 8 swallowed `Err(_)`
   and 16 discarded `.ok();` sites listed under rule 26.
3. Rule 38: inject a clock into ui/sidebar/daemon_supervisor/transport (about 25 `Instant::now()` sites) and a file
   port for the 30 `std::fs` files; the tests that need real files then use the fake.
4. Rule 7: rewrite or delete the 23 non-spec `SPEC:` comments listed.
5. Rule 28: split SidebarData (11), TextAreaSurface (10), Platform (8), TextSurface (7), PairingSource (6),
   SettingsBackend (5), FolderSource (5) into traits of at most 3 methods; merge the two `Clock` traits into one crate.
6. Rule 36: one verifier implementation shared by transport and mobile/src-tauri (4 copies).
7. Rule 34: split the `bool`-returning mutators listed under rule 34 into command plus query.
8. Rule 31/45: split settings_state.rs, terminal.rs, daemon_supervisor.rs, shell_state.rs, settings_core.rs, tab.rs,
   terminal_view_mouse.rs, split_node.rs, pane_host.rs by responsibility, and fold the `_mouse/_paint/_ime` family
   into real types instead of `impl` blocks scattered over files.
9. Rule 41: replace loopback sockets and tempdirs in unit tests with in-memory fakes; move the rest to `tests/` with `#[ignore]`.
10. Rules 3/4/6: the 22 `match bool` arms and 11 for+push accumulators listed.
11. settings_modal.rs:18: five parameters, make a struct.
12. Rule 13/14: make fields private in the pub structs listed; trim ui, sidebar, settings_ui `pub use` lists.
13. Rule 32: hex/underscore literals outside generated palettes (550) and magic strings in catalog files
    (settings_ui/catalog/*.rs, ui/icon_files.rs, shell_keys/command_table.rs are tables and acceptable if const).

Swift (script clean; all items below are review findings):
- ChromePlugin.swift:17-117: one helper for the main-queue + `#available` + `invoke.resolve()` wrapper (13 copies).
- SettingsModel.swift:7,13 and ChromeOverlays.swift:165: `try?` swallows decode errors; name the `"system"` default;
  Dialogs.swift:66 `"Cancel"` constant.
- NavigationController.swift:101-112: named constants for 0.4, 0.6 and the finger counts.
- Constructor-inject `send`/`present`/`showTab` in ChromeModel, SettingsModel, TabsModel, HostPickerModel,
  ModelPickerModel, NavigationController, DrawerSwipe, PageChannel, StripCatcher, KeyboardFit; AppScheme.swift:49 take a
  `URLSession` in init.
- 85 Swift sources have no test (115 sources vs 30 tests); reduce `swift-test-exemptions.txt` and cover state types.

TypeScript:
- sdk/pane-client.ts: `setDirty(dirty: boolean)` -> `markDirty()` / `markClean()`; extract the timer/pending part of
  `call()` (depth at 127-138); validate JSON in `receive` instead of casting.
- entries/buffer-pane-host.tsx:28: split the component (55 lines); lines 40 SPEC comment.
- entries/follow-shell-appearance.ts:12 delete comment; lib/ws/shell-socket.ts:8,14 (doc on private, name 200);
  __tests__/lib/ws/shell-socket.test.ts it.each; __tests__/entries/buffer-pane-host.test.tsx:58 depth.
- features/settings/lib/shell-settings.ts: derive SHELL_KEYS from the Rust PANE_SETTING_KEYS (generate it) rather than a
  second copy; `writeSettingToShell` split into `canWriteToShell(key)` plus a void send; module-level `useShellRoute` into an
  injected object.
- lib/ws/channel-handlers.ts:handleOpen split by responsibility; lib/ws/open-channel.ts `park()` side effects; rename `ch`,
  `cb`.
- Legacy: 1,738 files / 19,044 findings under app/panes are exempt; list and burn-down policy is the owner's call
  (tools/scripts/standards-burndown.sh writes the per-package table). The 3 TS import cycles listed under rule 20 are
  the cheapest legacy fixes.

Tooling (so the next audit is honest):
- brace-rules.awk: take the parameter list from the `(` after the function name, not the first `(` of the line; detect
  tuple-pattern parameters and count their elements; accept hex/underscore literals in magic-number; add closure bool params.
- rust-rules.awk: reject `SPEC:`/`PLATFORM:` comments without a URL or RFC; require `CASES`/`cases` as an identifier
  not a word; flag `unreachable!`; extend `real-infra` to sockets, tempfile, std::fs.
- check-cycles.py: walk subdirectories and `super::`/`self::` paths.
- check-ts-rules.sh: `layer` must resolve relative imports; run the TS checks on push and draft PRs too; add
  `app/mobile/src-tauri` to the Rust roots.
- Run `cargo clippy`, `cargo machete`, swiftlint, swift-format, eslint, knip once so the "clippy covers it" column of
  standards-coverage.md is evidence rather than claim.
