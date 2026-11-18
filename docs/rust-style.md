# Rust style manifest

Applies to every Rust crate under `app/` (`app/crates`, `app/desktop`, `app/mobile/ios-plugin`, `app/mobile/src-tauri`). Vendored code (`app/desktop/vendor`) is exempt. It is the Rust translation of the owner's Rabbyte manifest: the language-independent rules carry over, the Go-only rules are replaced by the Rust mechanism that does the same job. Swift: `docs/swift-style.md`. Review: `docs/plans/2026-11-14-code-review-checklist.md`.

Enforcement tags: **fmt** = `cargo fmt --check` (`app/rustfmt.toml`); **lint** = `cargo clippy --workspace --all-targets -- -D warnings` with the lint table below; **script** = `tools/scripts/check-rust-rules.sh`; **deny** = `cargo deny check` (`app/deny.toml`); **review** = reviewer agent with the checklist.

Sources: [Rust API Guidelines](https://rust-lang.github.io/api-guidelines/checklist.html) (C-xxx ids below), [Rust Style Guide](https://doc.rust-lang.org/style-guide/), [Clippy lint list](https://doc.rust-lang.org/clippy/lints.html) and [configuration](https://doc.rust-lang.org/clippy/lint_configuration.html), [rustc lints](https://doc.rust-lang.org/rustc/lints/listing/allowed-by-default.html), [Rust Book ch.7 modules](https://doc.rust-lang.org/book/ch07-00-managing-growing-projects-with-packages-crates-and-modules.html), [rustdoc book](https://doc.rust-lang.org/rustdoc/how-to-write-documentation.html), [cargo-deny book](https://embarkstudios.github.io/cargo-deny/), Clean Code (R. Martin) for rules 29-38.

## Rules

Numbers match the owner manifest (M#) so a finding can cite either.

### I. Code

| # | Rule | Enforcement |
|---|------|-------------|
| M1 | Parameters: one per line, always (two or more parameters; `(` on the signature line, `)` on its own line). Functions take at most 4 parameters; more means a struct (C-BUILDER). | fmt (`fn_params_layout = "Vertical"`), lint `too_many_arguments` (threshold 4); script `param-layout`, `param-count` |
| M2 | At most 2 nesting levels inside a function body (the body is level 1, a block inside it level 2, a block inside that is rejected). `match` itself adds no level (arms are aligned with it, like Go `switch`); an arm block does. The `mod tests` wrapper is not a level: depth is counted from the function. | script `depth` (authority); lint `excessive_nesting` (threshold 3, a backstop: clippy counts the fn body as depth 1), `cognitive_complexity` <= 10 |
| M3 | Guard clauses and early returns; no `else` at all (`let ... else` is a guard and allowed). Use `?`, `matches!`, early `return`. | script `else`; lint `redundant_else`, `manual_let_else`, `question_mark`, `collapsible_else_if` |
| M4 | (Go named returns) Rust analog: no mutable accumulator (`let mut result/out/acc ...`) returned at the end; return a named expression or type, never an anonymous tuple of more than two fields; no out-parameters (C-NO-OUT). Rust has no naked returns. | script `named-return`; lint `let_and_return`, `type_complexity`, `needless_pass_by_ref_mut` |
| M5 | A compound condition (two or more `&&`/`||`) is bound to a named `let`/`fn` before the `if`. | script `long-condition`; lint `nonminimal_bool` |
| M6 | Prefer iterator adapters over hand loops (`map`, `filter`, `find`, `any`, `fold`, `collect`). | lint `needless_range_loop`, `manual_filter_map`, `manual_find`, `manual_flatten`, `explicit_iter_loop` (all in `all`/`pedantic`) |
| M7 | Comments: no inline narrative `//` comments, no doc comments on private items, a doc comment on every `pub` item (summary line, `# Errors`, `# Panics`, `# Safety`, example where it clarifies; C-CRATE-DOC, C-FAILURE, C-LINK). The only allowed `//` comments start with `SAFETY:`, `SPEC:` (link to a spec or RFC), `PLATFORM:` (platform constraint) or `DEPRECATED:`. | lint `missing_docs`, `missing_errors_doc`, `missing_panics_doc`, `missing_safety_doc`, `undocumented_unsafe_blocks`; script `comment`, `doc-private` |

### II. Types

| # | Rule | Enforcement |
|---|------|-------------|
| M8 | One type (`struct`, `enum`, `trait`, `union`) per file, in a file named after it in `snake_case` (`HostToken` in `host_token.rs`). A type alias, constants and the type's `impl` blocks live with it. An error type lives in `error.rs` as `Error` (path `transport::Error`, C-GOOD-ERR). | script `multi-type`, `type-name`; lint `module_name_repetitions` |
| M9 | Errors are a `thiserror` enum named `Error` in `error.rs`; variants carry the source (`#[from]`/`#[source]`) and context fields. `anyhow` only in binary crates (`app/desktop/app`). | lint `error_impl_error` is not enabled on purpose; script `cargo-anyhow`; review |
| M10 | A crate's entry function or entry type lives in a file named after the crate (`transport.rs` in crate `transport`); `lib.rs` contains only `mod`, `pub use` and crate docs (`//!`). `main.rs` stays thin (parse, build, run). | script `lib-decl`; review |
| M11 | A type with no state (no fields, or only unit use) is a free function in a module, not a struct with methods. | script `stateless-type`; lint `unused_self`; review |
| M12 | `new()` exists only when the type has real state to validate or establish (a parameterless `new()` is rejected); otherwise a struct literal or `Default` (C-CTOR). Fallible construction returns `Result`. | script `new-stateless`; lint `new_without_default`, `new_ret_no_self` |
| M13 | The public API exposes traits (or concrete types with private fields, C-STRUCT-PRIVATE) and keeps implementations private; sealed traits where downstream impls must be blocked (C-SEALED). | lint `unreachable_pub`; review |

### III. Modules (Rust form of Go `internal/`)

| # | Rule | Enforcement |
|---|------|-------------|
| M14 | `lib.rs` exposes only the entry, the `Error`, the traits and the types that appear in their signatures, via `pub use`. | lint `unreachable_pub`; review |
| M15 | Everything else is a private `mod` or `pub(crate)`; sub-modules are never `pub mod` unless they are the API. Prefer `pub(crate)` over `pub` for crate-internal sharing. | lint `unreachable_pub`; script `mod-rs` (modern `foo.rs` + `foo/` layout only, lint `mod_module_files`) |
| M16 | Peer crates talk through each other's public API only. Crates in the same layer never depend on each other. Shared crates (`app/crates/*`) never depend on `gpui*`, `tauri*`, `wry` or `gtk*`, nor on anything under `app/desktop`. | script `cargo-layer`; review |
| M17 | Dependency direction is one way, down the five layers below; a crate may skip layers downward. Crates in the same layer never depend on each other, and logic crates never depend on `gpui` or on a gpui crate (only `ui`, the L3 views and the binary do). Every crate is listed in `tools/scripts/rust-layers.txt`; a crate missing from it fails. A back-edge is wrong placement. | script `cargo-layer` (`check-layers.py` over `rust-layers.txt`); cargo rejects cycles |
| M18-19 | (Go internal/ semantics, import cycles) covered by M15, M17 and by Cargo, which refuses dependency cycles. | cargo |
| M20 | No cycles between modules inside a crate. | lint `cyclic_dependencies` is not in clippy; review of `mod` graph |
| M21 | Crate names: one lowercase word, no `_` or `-` (`atomicfile`, `windowstate`; two words are joined only when one cannot name the job). Module names: short, lowercase `snake_case`. No stutter (`transport::Client`, not `transport::TransportClient`). | lint `module_name_repetitions`; C-CASE |
| M22 | No module, file or crate named `util(s)`, `common`, `helper(s)`, `base` or `misc`. Name the concept. | script `module-name` |
| M23 | Re-exports and names read the same from the crate root (C-WORD-ORDER). | review |
| M24 | No global mutable state, no init magic: no `static mut`, `lazy_static!`, `thread_local!`, `OnceLock`/`LazyLock`/`Mutex`/`Atomic*` statics, no `ctor`/`inventory`. State is passed in. | script `global`; lint `static_mut_refs`; deny bans |
| M25 | Cancellation and deadlines are explicit: async fns that can block take a `CancellationToken` or are cancel-safe and documented so; no blocking calls on async threads. | lint `await_holding_lock`, `large_futures`; review |
| M26 | Errors are values: return `Result`, never swallow. No `let _ =` on a `Result`, no `.ok()` to discard, no `unwrap_or_default` hiding failure. | lint `let_underscore_must_use`, `unused_must_use`, `map_err_ignore`, `unwrap_used` |
| M27 | Wrap errors with context at every layer boundary (`#[source]` + a field saying what was attempted). No `panic!`, `unwrap`, `expect`, `todo!`, `unimplemented!`, `dbg!`, indexing that can panic, outside tests and `main.rs`/`src/bin`. Startup in `main.rs` may `expect` with a message. | lint `unwrap_used`, `expect_used`, `panic`, `todo`, `unimplemented`, `dbg_macro`, `indexing_slicing`, `exit`; script `panic` |
| M28 | Traits are small and defined where they are consumed; the implementer depends on the consumer's trait, not the reverse. | review |

### V. Clean code

| # | Rule | Enforcement |
|---|------|-------------|
| M29 | Intent-revealing names; no abbreviations, no `tmp`/`data`/`info` (`foo`, `baz`, `tmp` rejected). | lint `disallowed_names`; review |
| M30 | A function does one thing and is under 40 lines. | lint `too_many_lines` (threshold 40), `cognitive_complexity`; script `fn-length` |
| M31 | Single responsibility per type and module (M8 makes it visible). | review |
| M32 | No magic numbers or strings: named `const` or a config field with a doc. Numbers other than 0, 1, 2, 10, 100 outside `const`/`static` and tests are rejected; magic strings are found by sampled review. | script `magic-number`; review |
| M33 | No `bool` flag parameters: use an enum or two functions (C-CUSTOM-TYPE). At most 2 bools per struct. | script `bool-param`; lint `fn_params_excessive_bools` (max 0), `struct_excessive_bools` (max 2) |
| M34 | Command-query separation: a fn either returns a value or changes state, not both; getters take `&self` (C-GETTER). | review; lint `needless_pass_by_ref_mut` |
| M35 | No hidden side effects: a fn named like a query does no I/O; I/O sits behind a trait. | review |
| M36 | Duplicate once, extract on the third repetition. | review |
| M37 | Dead code is deleted in the same change. No `#[allow(dead_code)]`, no unused deps (`cargo machete`). | lint `dead_code`, `unused`; script `allow-dead`; `cargo machete` |
| M38 | Depend on abstractions; collaborators are constructor arguments or generic parameters, never fetched from globals (C-GENERIC, C-CALLER-CONTROL). | review |

### VI. Tests

| # | Rule | Enforcement |
|---|------|-------------|
| M39 | Table-driven: a `const CASES: &[(...)]` or array of structs and one loop, one behaviour per case. A test module with more than two `#[test]` fns and no `CASES`/`cases` table is rejected. | script `table-driven` |
| M40 | Test the public interface: unit tests sit in the file's `mod tests` and assert observable behaviour of the file's public items; integration tests in `tests/` use the crate API only. | review (sampled) |
| M41 | No real infrastructure by default (no network, daemon, display, home directory, child process). Fakes in `tests/support/` or `src/test_support/`. Tests needing a real service are `#[ignore = "needs <thing>"]` and run by an explicit CI job. A test that inherently needs a real filesystem, socket or process (the adapter integration boundary; anything a fake can cover uses the fake) is opt-in: `#[ignore = "real-infra: <what and why>"]` (reason of three or more words) on the test and on any helper only such tests call; the script exempts the body of a fn so marked, and any file under a `real_infra/` directory of test support (`tests/support/real_infra/`). The `Real-infrastructure tests` CI step runs them with `--include-ignored`. No `sleep`; wait on channels, `tokio::time::pause`, or a deadline loop. | script `real-infra`; lint `disallowed_methods` for sleep |

### VII. Files

| # | Rule | Enforcement |
|---|------|-------------|
| M42 | 1:1 tests: every source file `src/foo.rs` that contains a function holds its unit tests in `#[cfg(test)] mod tests { use super::*; ... }` at the bottom of that same file. Glue files with no logic are listed in `tools/scripts/rust-test-exemptions.txt`, which only shrinks. | script `test-missing` |
| M43 | No free test files: `*_tests.rs`, `tests.rs`, `#[path]` wiring, `#[test]` outside the file's `mod tests`, and any `#[cfg(test)]` other than `mod tests {` / `mod test_support;` are rejected. `tests/<concern>.rs` is allowed only for public-API integration tests and needs `src/<concern>.rs` or the crate name. | script `test-sibling`, `test-in-src`, `cfg-test`, `test-orphan` |
| M44 | A source file is at most **500 lines**, counted without blank lines, doc comments and the `#[cfg(test)] mod tests` block. No exceptions. | script `size` |
| M45 | A file that outgrows 500 lines is the wrong abstraction: split by concept (M8), never by trimming comments or lines. | review |

## Crate layers (M16, M17)

The map of `docs/plans/2026-11-18-rust-tree.md` section 1. `tools/scripts/rust-layers.txt` holds the same table for the `cargo-layer` check.

| Layer | Kind | Crates |
|---|---|---|
| L4 binary | composes everything below | `crowbar` |
| L3 gpui views | gpui | `sidebarview`, `settingsview`, `terminalview`, `layoutview`, `viewtabs`, `ideshell`, `browserbar`, `startup`, `oobe`, `pairing`, `branchimport`, `folderpicker` |
| L2 services | logic | `panehost`, `store`, `ptysession` |
| L1 domain | logic | `layout`, `transport`, `browseragent`, `keymap`, `windowstate`, `daemonapi`, `webview`, `sidebar`, `settings` |
| L0 foundation | logic, except `ui` (gpui) | `protocol`, `atomicfile`, `browser`, `terminal`, `supervisor`, `diagnostics`, `platform`, `chord`, `ui` |

`unsafe` lives only in `app/desktop/crates/platform/src/linux/` and `src/macos/`, each block behind a safe API and listed in the **Unsafe** section of `app/desktop/crates/platform/README.md`; the iOS plugin's C ABI file `app/mobile/ios-plugin/src/ffi.rs` is the one exception (script `unsafe-home`).

## The test-file mapping (decision of 2026-11-15)

```
crates/transport/src/host_token.rs        the code, with `#[cfg(test)] mod tests { use super::*; ... }` at the bottom
crates/transport/tests/transport.rs       public-API tests of the crate (needs src/transport.rs or the crate name)
crates/transport/tests/support/*.rs       shared fakes and fixtures for tests/
crates/transport/src/test_support/*.rs    shared fakes for unit tests (declared once as `#[cfg(test)] mod test_support;` in lib.rs)
```

The unit tests of a file live in that file, so the test is always beside the code it covers and reaches its private items. The 500-line ceiling does not count the test module. Test code is still subject to the structure rules (depth, parameters, comments, `else`); only `unwrap`/`expect`/`panic` and magic numbers are allowed in tests (`clippy.toml` `allow-*-in-tests`, script test regions).

Exceptions, all in the script: `lib.rs` is declaration-only; `main.rs`, `build.rs`, `src/bin/*.rs` are thin entry points; a file with no `fn` needs no tests; `rust-test-exemptions.txt` lists glue files (GPUI view wiring) that cannot be unit tested, and only shrinks. `tools/scripts/inline-rust-tests.sh DIR...` migrates sibling `*_tests.rs` files mechanically.

## Public surface

`unreachable_pub = deny` makes `pub` mean "reachable from the crate root". A `pub` item that cannot be reached must be `pub(crate)`. `lib.rs` re-exports the entry, `Error`, traits and the types that appear in their signatures (M14); all `mod` declarations are private. Public types have private fields (C-STRUCT-PRIVATE), implement `Debug` (`missing_debug_implementations`), and public enums that may grow are `#[non_exhaustive]`. Public items of the shared crates are documented; `pub(crate)` and private items are not.

## Lint table for `app/Cargo.toml`

Owner of `app/Cargo.toml` pastes this verbatim; every member crate then has `[lints] workspace = true`. Thresholds live in `app/clippy.toml`. An exception is a crate-level `#![allow(lint, reason = "...")]` or `#[expect]` with a reason (script `allow`), e.g. `unsafe_code` in `platform`, the one crate that holds `unsafe`.

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

## How to run

```
tools/scripts/check-rust-rules.sh              # structure rules over app/
tools/scripts/check-rust-rules.sh --self-test  # checker against its fixtures
tools/scripts/check-rust-rules.sh --full       # also fmt, clippy, deny, machete when installed (missing tool = failure)
tools/scripts/check-standards.sh              # Rust + Swift + TypeScript in one run
```

`check-standards.sh` is a required CI job; `--full` also runs on the Linux and macOS runners.

## Notes

- `redundant_pub_crate` and `missing_const_for_fn` are not enabled: the first conflicts with `unreachable_pub`, the second is noise.
- `pub_use`, `single_call_fn`, `multiple_inherent_impl` stay off: re-exports are the API mechanism (M14).
- `excessive_nesting` counts nested blocks inside a function; verify the threshold in `app/clippy.toml` against M2 on the first compile and adjust the number, never the rule.
