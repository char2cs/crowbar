# Rust workspace crates

`app/Cargo.toml` is the workspace root. Shared crates (no GPUI, no platform code) live here in `app/crates`; desktop crates live in `app/desktop/crates`, the binary in `app/desktop/app`, patched third-party crates in `app/desktop/vendor` (see its README). `app/mobile` is a separate workspace.

Rules for every crate: `docs/rust-style.md` and `docs/rabbyte-manifest.md`. Check with `tools/scripts/check-rust-rules.sh` and `tools/scripts/check-standards.sh`; CI is `.github/workflows/rust.yml`.

## workspace lints

Paste this table verbatim into `app/Cargo.toml` (it is the single source: `docs/rust-style.md`, section "Lint table"), and give every member crate `[lints] workspace = true`. Crates outside the workspace (`app/mobile/ios-plugin`, `app/mobile/src-tauri`) carry a copy of the same table with `[workspace.lints.X]` renamed to `[lints.X]`. Thresholds are in `app/clippy.toml`, formatting in `app/rustfmt.toml`, licenses and bans in `app/deny.toml`.

```toml
[workspace.lints.rust]
rust_2018_idioms = { level = "deny", priority = -1 }
future_incompatible = { level = "deny", priority = -1 }
nonstandard_style = { level = "deny", priority = -1 }
unused = { level = "deny", priority = -1 }
dead_code = "deny"
missing_docs = "deny"
unreachable_pub = "deny"
missing_debug_implementations = "deny"
unused_must_use = "deny"
unused_qualifications = "deny"
unsafe_code = "deny"
unsafe_op_in_unsafe_fn = "deny"
static_mut_refs = "deny"

[workspace.lints.rustdoc]
broken_intra_doc_links = "deny"
private_intra_doc_links = "deny"

[workspace.lints.clippy]
all = { level = "deny", priority = -1 }
pedantic = { level = "deny", priority = -1 }
must_use_candidate = "allow"
similar_names = "allow"
unwrap_used = "deny"
expect_used = "deny"
panic = "deny"
todo = "deny"
unimplemented = "deny"
dbg_macro = "deny"
print_stdout = "deny"
print_stderr = "deny"
exit = "deny"
mem_forget = "deny"
indexing_slicing = "deny"
string_slice = "deny"
let_underscore_must_use = "deny"
undocumented_unsafe_blocks = "deny"
multiple_unsafe_ops_per_block = "deny"
allow_attributes_without_reason = "deny"
mod_module_files = "deny"
missing_assert_message = "deny"
clone_on_ref_ptr = "deny"
rc_mutex = "deny"
same_name_method = "deny"
str_to_string = "deny"
empty_drop = "deny"
cognitive_complexity = "deny"
excessive_nesting = "deny"
large_futures = "deny"
await_holding_lock = "deny"
await_holding_refcell_ref = "deny"
```

## layout per crate

```
src/lib.rs               mod declarations, pub use, //! docs only
src/<crate>.rs           the entry (function or type) named after the crate
src/<type>.rs            one type per file, named after it, with its unit tests in a
                         #[cfg(test)] mod tests at the bottom
tests/<crate>.rs         public-API tests; tests/support/ shared fakes
```

## crates

| crate | owns |
| --- | --- |
| `protocol` (shared) | control frames (`sdk@1` JSON text), lifecycle, ids, the 64 KiB `PaneState` cap, `AppPhase`, `DaemonTransport` |
| `model` (shared) | tabs, pane slots, the split tree, pane instances and `DragSubject` as a pure state machine with undo |
| `transport` (shared) | `ShellTransport`, the implementation of `DaemonTransport`: HTTP to the daemon, paired hosts and tokens, certificate pinning, the WebSocket relay |
| `browser_nav` (shared) | address-bar interpretation, which URLs a browser pane may load, page state for the native chrome |
| `webview` | the only platform crate: create, place, show, hide, eval and destroy web views; GTK thread on Linux |
| `pane_host` | pane registry, slot pool, park and restore, focus routing, overlay hiding, the `crowbar://` protocol |
| `daemon_supervisor` | find, spawn, health-check, recover and stop the daemon sidecar (desktop only; iOS has no sidecar) |
| `ui` | GPUI design system: theme tokens, fonts, icons, motion (`Glide`), `DragGhost`, `Overlays`, base components |
| `tab_bar` | the tab strip: pills, reorder and merge drags, drop line, context menu |
| `layout_view` | the pane layout: frames, handles, accent ring, sashes, drop zones; `pane_holes` gives the web view rectangles |
| `sidebar` | frame, header, space switcher, mode bar, chats tree, files and changes panels |
| `settings_ui` | schema-driven native settings modal |
| `startup_view` | full-screen startup and data-migration view |
| `command_palette` | fuzzy matcher over command titles and the list view |
| `terminal` / `terminal_view` | terminal emulation (alacritty grid, daemon frame feed, selection, search, key encoding) without GPUI, and its GPUI view |
| `shell_data` | daemon-backed data for the shell: sidebar entities and commands, settings and records |
| `shell_keys` | which chords the shell owns and how a key typed in a pane reaches it |
| `shell_platform` | open a link in the system browser, reveal a path, the rule for where a link may go |
| `shell_diagnostics` | rotated log file and a report file for every panic |
| `window_state` | remembers window size, position and maximized state |
| `crowbar_desktop` (`desktop/app`) | the binary: GPUI window, native chrome regions, wiring of all of the above |

## wiring

- `pane_host::PaneProtocol::new(scheme, transport, bundles, extension_bundles, origins)` takes an `Arc<dyn protocol::DaemonTransport>`; `crowbar_desktop` passes `transport::ShellTransport`. Every `crowbar://<host>/v0/...` request from a pane becomes a `DaemonRequest` answered through the `Responder`; `relay_endpoint` serves WebSocket access.
- `tab_bar` and `layout_view` render the model and issue `model::Command`s. The app applies them to `Layout` and pushes the new state back (`TabBar::set_tabs`, `LayoutView::set_tab`); neither keeps layout state of its own.
- `LayoutView::new(Rc<dyn PaneSource>)`: the app implements `PaneSource`. Pane content is an empty element because the web view covers the hole.
- `layout_view::pane_holes(tab, content, ChromeContext)` returns the inset hole rect of every pane; the app builds `pane_host::LayoutSnapshot::from_layout(..)` and replaces each `visible` bound with the matching hole.
- Web views are native child windows, so GPUI never sees the pointer over them: the app calls `LayoutView::set_hovered_pane` from the pane host's hover notifications. `ui::Overlays` hides web views while a drag is over the layout and while the pane handle is shown.

## reuse by mobile

`transport` depends on neither gpui nor tauri, so `app/mobile/src-tauri` can use it by path instead of its own `api_proxy`, `hosts`, `host_tokens`, `tls_pin` and `ws_bridge`. Mobile is not rewired yet; the originals stay until it is. What mobile supplies: a `SocketLocator`, a `TokenStore` (`OsKeychain` behind the `os-keychain` feature, or its own), an `OriginPolicy` (`ExactOrigins` with the iOS webview origins), and either a `ShellTransport` or `Transport::handle` called from its custom protocol handler, plus a `Relay` if its panes open WebSockets.

## known gaps

- Tabs: no pinning or compact pins, no label fade mask under the close button (ellipsis), a tab drag out of the strip onto a pane is not handled, the tab context menu has Close and Close Others only.
- Layout: no inset shadows; cross-project drops are not refused (needs a project id on pane instances).
- Fonts: no italic face (the provisional chat title is italic in the reference and GPUI does not synthesise one) and no emoji font in the app assets, so `Emoji` project marks render as tofu in a container without system emoji fonts.

## requests

- Pane relay for another host: a pane gets `__CROWBAR__.ws` for its own host only, so `createTransport` in `app/panes/src/lib/ws/manager.ts` refuses every remote host. `ShellTransport::relay_endpoint` already issues one token per `RelayScope` (host, pane, origin) and refuses unregistered hosts. What is missing is a way for a pane to ask: add `PaneFrame::RelayOpen { id, host }` answered by a `ShellFrame::Response` whose result is the `RelayEndpoint` `{ url, token }` for `RelayScope::new(host, <sender pane>, <sender origin>)`, built by the pane host from the webview's own identity, never from the frame. The pane side then caches one endpoint per host in `PaneClient` and `openShellSocket` takes the endpoint instead of reading `__CROWBAR__.ws`.
- Theme frame: the chat pane reads `theme-id` and `theme-type` (`light` or `dark`) tokens; the shell sends `ShellFrame::Theme` after `Mount` and on every theme change.
