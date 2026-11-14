# Shared Rust crates

Rules for every crate here: `docs/rust-style.md`. Check with `tools/scripts/check-rust-rules.sh`.

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
src/<type>.rs            one type per file, named after it
src/<type>_tests.rs      all unit tests of <type>.rs, wired with #[cfg(test)] #[path = "<type>_tests.rs"] mod tests;
tests/<crate>.rs         public-API tests; tests/support/ shared fakes
```

## reuse by mobile

`transport` depends on neither gpui nor tauri, so the Tauri crate in `app/mobile/src-tauri` can use it by path instead of its own copies of `api_proxy`, `hosts`, `host_tokens`, `tls_pin` and `ws_bridge`. Mobile is not rewired yet; the originals stay until the owner moves it. What mobile needs to supply:

- a `SocketLocator` (a fixed `PathBuf` is enough) and a `TokenStore` (`OsKeychain` behind the `os-keychain` feature, or its own);
- an `OriginPolicy` (`ExactOrigins` with the iOS webview origins);
- a `ShellTransport` (for the `DaemonTransport` trait) or `Transport::handle` called straight from its custom protocol handler, and a `Relay` if its panes open WebSockets.

`daemon_supervisor` (`app/desktop/crates/daemon_supervisor`) is desktop only: iOS has no sidecar.

## crate ownership

| crate | owns |
| --- | --- |
| `protocol` | control frames (`sdk@1` JSON text), lifecycle, ids, the 64 KiB `PaneState` cap, `AppPhase`, `DaemonTransport` |
| `model` | tabs, pane slots, the split tree and pane instances as a pure state machine with undo |
| `transport` | `ShellTransport`, the implementation of `DaemonTransport` (HTTP to the daemon, WebSocket relay) |
| `webview` (desktop) | the only platform crate: create, place, show, hide, eval and destroy web views; GTK thread on Linux |
| `pane_host` (desktop) | pane registry, slot pool, park and restore, focus routing, overlay hiding, the `crowbar://` protocol |
| `tab_bar` (desktop) | the tab strip: pills, reorder/merge drags, drop line, context menu, collapsed-sidebar toggle; emits `TabBarEvent::Apply(Vec<Command>)` |
| `layout_view` (desktop) | the pane layout: frames, handles, accent ring, sashes, drop zones and preview; emits `LayoutEvent::Apply(Command)` |
| `crowbar_desktop` (desktop/app) | the thin binary: GPUI window, native chrome regions, one real chat pane |

## DaemonTransport for pane_host

`pane_host::PaneProtocol::new(scheme, transport, bundles, extension_bundles, origins)` takes an `Arc<dyn protocol::DaemonTransport>`. Every `crowbar://<host>/v0/...` request from a pane becomes a `DaemonRequest` and is answered through the `Responder`; `relay_endpoint` serves WebSocket access. Pass `transport::ShellTransport` there. `crowbar_desktop` uses a loopback stand-in (`/v0/health` only) until it is wired.

## tab_bar and layout_view integration

Both views render the model and issue `model::Command`s; the app applies them to `Layout` and pushes the new state back (`TabBar::set_tabs`, `LayoutView::set_tab`). Neither keeps layout state of its own.

- `LayoutView::new(Rc<dyn PaneSource>)`: the app implements `PaneSource` (hole content, drag-ghost label, a fresh `TabId` for detach). Content is an empty element in production because the pane host's web view covers the hole.
- `layout_view::pane_holes(tab, content, ChromeContext)` returns the inset hole rect of every pane. The app builds `pane_host::LayoutSnapshot::from_layout(..)` and replaces each `visible` bound with the matching hole, so web views stop at the pane border and radius.
- Web views are native child windows, so GPUI never sees the pointer over them: the app calls `LayoutView::set_hovered_pane` from the pane host's hover notifications (the handle fades in from it).
- `ui::Overlays` hides web views while a drag is over the layout and while the pane handle is shown.
- Shared drag payload: `model::DragSubject` (`Pane`, `Open(PaneInstance)`, `Tab`). The sidebar starts `Open`/`Tab` drags with it; tab_bar and layout_view accept it.
- Added to `ui`: `Glide` (retargetable eased value, honours reduced motion) and `DragGhost` (the 32px lifted pill).
- Demos: `layout_demo` and `tab_bar_demo` binaries (fake model, no daemon).

Known gaps: no tab pinning or compact pins, no label fade mask under the close button (ellipsis), no inset shadows, tab drag out of the strip onto a pane is not handled, cross-project drops are not refused, and the tab context menu has Close and Close Others only.

## requests to protocol/model owners

- `model`: a per-tab `pinned` flag (and a `Command` to set it) for tab pinning.
- `model`: per-pane titles so the strip can show chat names without the shell passing them in.
- `model`: a project id on pane instances, so a drop across projects can be refused.
- `ui`: an italic font face (or synthetic italic): the provisional chat title is italic in the reference and GPUI does not synthesise one from the upright faces.
- `ui`: an emoji font in the app assets; project marks of kind `Emoji` render as tofu in a container without system emoji fonts.
- `ui`: the sidebar added icons (`ArrowsOutCardinal`, `XCircle`, `Funnel`, `UploadSimple`, `PencilSimple`, `SidebarToggle`, `SidebarToggleRight`), `TextInputStyle::Bare` and `TabItem::with_label`.
