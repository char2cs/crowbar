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
