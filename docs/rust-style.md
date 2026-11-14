# Rust style manifest

Applies to every Rust crate under `app/` (`app/crates`, `app/desktop`, `app/mobile/ios-plugin`, `app/mobile/src-tauri`). Vendored code (`app/desktop/vendor`) is exempt. It is the Rust translation of the owner's Rabbyte manifest: the language-independent rules carry over, the Go-only rules are replaced by the Rust mechanism that does the same job. Swift: `docs/swift-style.md`. Review: `docs/plans/2026-11-14-code-review-checklist.md`.

Enforcement tags: **fmt** = `cargo fmt --check` (`app/rustfmt.toml`); **lint** = `cargo clippy --workspace --all-targets -- -D warnings` with the lint table below; **script** = `tools/scripts/check-rust-rules.sh`; **deny** = `cargo deny check` (`app/deny.toml`); **review** = reviewer agent with the checklist.

Sources: [Rust API Guidelines](https://rust-lang.github.io/api-guidelines/checklist.html) (C-xxx ids below), [Rust Style Guide](https://doc.rust-lang.org/style-guide/), [Clippy lint list](https://doc.rust-lang.org/clippy/lints.html) and [configuration](https://doc.rust-lang.org/clippy/lint_configuration.html), [rustc lints](https://doc.rust-lang.org/rustc/lints/listing/allowed-by-default.html), [Rust Book ch.7 modules](https://doc.rust-lang.org/book/ch07-00-managing-growing-projects-with-packages-crates-and-modules.html), [rustdoc book](https://doc.rust-lang.org/rustdoc/how-to-write-documentation.html), [cargo-deny book](https://embarkstudios.github.io/cargo-deny/), Clean Code (R. Martin) for rules 29-38.

## Rules

Numbers match the owner manifest (M#) so a finding can cite either.

### I. Code

| # | Rule | Enforcement |
|---|------|-------------|
| M1 | A signature or call that does not fit on one line puts one parameter or argument per line. Functions take at most 4 parameters; more means a struct (C-BUILDER). | fmt (`fn_params_layout = "Tall"`), lint `too_many_arguments` (threshold 4) |
| M2 | At most 2 nesting levels inside a function body. | lint `excessive_nesting` (threshold in `clippy.toml`), `cognitive_complexity` <= 10 |
| M3 | Guard clauses and early returns; no `else` after a branch that leaves. Use `let ... else`, `?`, `matches!`. | lint `redundant_else`, `manual_let_else`, `if_not_else`, `needless_continue`, `question_mark`, `collapsible_else_if` |
| M4 | (Go named returns) Replaced by C-CUSTOM-TYPE and C-NEWTYPE: return named types, never anonymous tuples of more than two fields; no out-parameters (C-NO-OUT). | lint `type_complexity`, `needless_pass_by_ref_mut`; review |
| M5 | A compound condition is bound to a named `let`/`fn` before the `if`. | lint `nonminimal_bool`; review |
| M6 | Prefer iterator adapters over hand loops (`map`, `filter`, `find`, `any`, `fold`, `collect`). | lint `needless_range_loop`, `manual_filter_map`, `manual_find`, `manual_flatten`, `explicit_iter_loop` (all in `all`/`pedantic`) |
| M7 | Comments: no inline narrative `//` comments, no doc comments on private items, a doc comment on every `pub` item (summary line, `# Errors`, `# Panics`, `# Safety`, example where it clarifies; C-CRATE-DOC, C-FAILURE, C-LINK). The only allowed `//` comments start with `SAFETY:`, `SPEC:` (link to a spec or RFC), `PLATFORM:` (platform constraint) or `DEPRECATED:`. | lint `missing_docs`, `missing_errors_doc`, `missing_panics_doc`, `missing_safety_doc`, `undocumented_unsafe_blocks`; script `comment`, `doc-private` |

### II. Types

| # | Rule | Enforcement |
|---|------|-------------|
| M8 | One type (`struct`, `enum`, `trait`, `union`) per file, in a file named after it in `snake_case` (`HostToken` in `host_token.rs`). A type alias, constants and the type's `impl` blocks live with it. An error type lives in `error.rs` as `Error` (path `transport::Error`, C-GOOD-ERR). | script `multi-type`, `type-name`; lint `module_name_repetitions` |
| M9 | Errors are a `thiserror` enum named `Error` in `error.rs`; variants carry the source (`#[from]`/`#[source]`) and context fields. `anyhow` only in binary crates (`app/desktop/app`). | lint `error_impl_error` is not enabled on purpose; script `cargo-anyhow`; review |
| M10 | A crate's entry function or entry type lives in a file named after the crate (`transport.rs` in crate `transport`); `lib.rs` contains only `mod`, `pub use` and crate docs (`//!`). `main.rs` stays thin (parse, build, run). | script `lib-decl`; review |
| M11 | A type with no state (no fields, or only unit use) is a free function in a module, not a struct with methods. | lint `unused_self`, `trivially_copy_pass_by_ref`; review |
| M12 | `new()` exists only when the type has real state to validate or establish; otherwise a struct literal or `Default` (C-CTOR). Fallible construction returns `Result`. | lint `new_without_default`, `new_ret_no_self`; review |
| M13 | The public API exposes traits (or concrete types with private fields, C-STRUCT-PRIVATE) and keeps implementations private; sealed traits where downstream impls must be blocked (C-SEALED). | lint `unreachable_pub`; review |

### III. Modules (Rust form of Go `internal/`)

| # | Rule | Enforcement |
|---|------|-------------|
| M14 | `lib.rs` exposes only the entry, the `Error`, the traits and the types that appear in their signatures, via `pub use`. | lint `unreachable_pub`; review |
| M15 | Everything else is a private `mod` or `pub(crate)`; sub-modules are never `pub mod` unless they are the API. Prefer `pub(crate)` over `pub` for crate-internal sharing. | lint `unreachable_pub`; script `mod-rs` (modern `foo.rs` + `foo/` layout only, lint `mod_module_files`) |
| M16 | Peer crates talk through each other's public API only. Crates in the same layer never depend on each other. Shared crates (`app/crates/*`) never depend on `gpui*`, `tauri*`, `wry` or `gtk*`, nor on anything under `app/desktop`. | script `cargo-layer`; review |
| M17 | Dependency direction is one way: `app/crates/*` <- `app/desktop/crates/*` <- `app/desktop/app`. A back-edge is wrong placement. | script `cargo-layer`; cargo rejects cycles |
| M18-19 | (Go internal/ semantics, import cycles) covered by M15, M17 and by Cargo, which refuses dependency cycles. | cargo |
| M20 | No cycles between modules inside a crate. | lint `cyclic_dependencies` is not in clippy; review of `mod` graph |
| M21 | Crate and module names: short, lowercase `snake_case`, no stutter (`transport::Client`, not `transport::TransportClient`). | lint `module_name_repetitions`; C-CASE |
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
| M30 | A function does one thing and is under 40 lines. | lint `too_many_lines` (threshold 40), `cognitive_complexity` |
| M31 | Single responsibility per type and module (M8 makes it visible). | review |
| M32 | No magic numbers or strings: named `const` or a config field with a doc. | review (no lint exists) |
| M33 | No `bool` flag parameters: use an enum or two functions (C-CUSTOM-TYPE). At most 2 bools per struct. | lint `fn_params_excessive_bools` (max 0), `struct_excessive_bools` (max 2) |
| M34 | Command-query separation: a fn either returns a value or changes state, not both; getters take `&self` (C-GETTER). | review; lint `needless_pass_by_ref_mut` |
| M35 | No hidden side effects: a fn named like a query does no I/O; I/O sits behind a trait. | review |
| M36 | Duplicate once, extract on the third repetition. | review |
| M37 | Dead code is deleted in the same change. No `#[allow(dead_code)]`, no unused deps (`cargo machete`). | lint `dead_code`, `unused`; script `allow-dead`; `cargo machete` |
| M38 | Depend on abstractions; collaborators are constructor arguments or generic parameters, never fetched from globals (C-GENERIC, C-CALLER-CONTROL). | review |

### VI. Tests

| # | Rule | Enforcement |
|---|------|-------------|
| M39 | Table-driven: a `const CASES: &[(...)]` or array of structs and one loop, `rstest`-style only if already a dependency. | review |
| M40 | Test the public interface. Unit tests in `foo_tests.rs` may use `use super::*` to reach the file's items, but assert on observable behaviour; integration tests in `tests/` use the crate API only. | review |
| M41 | No real infrastructure by default (no network, daemon, display, home directory). Fakes in `tests/support/` or `src/test_support/`. Tests needing a real service are `#[ignore = "needs <thing>"]` and run by an explicit CI job. No `sleep`; wait on channels, `tokio::time::pause`, or a deadline loop. | lint `disallowed_methods` is configured for `std::thread::sleep`/`tokio::time::sleep` in tests; review |

### VII. Files

| # | Rule | Enforcement |
|---|------|-------------|
| M42 | Every source file `src/foo.rs` that contains a function has exactly one test file beside it: `src/foo_tests.rs`, wired in `foo.rs` by the three lines below. All of `foo.rs`'s unit tests live there. | script `test-missing`, `test-unwired` |
| M43 | No free test files: a `*_tests.rs` without its `foo.rs`, `#[test]` inside a source file, an inline `mod tests { }`, `#[cfg(test)]` helpers in source, or a `tests/x.rs` without a source counterpart is rejected. | script `test-orphan`, `test-inline`, `test-in-src`, `cfg-test` |
| M44 | A source file is at most **500 lines**, no exceptions (tests, the fixture directory and vendored code excepted). | script `size` |
| M45 | A file that outgrows 500 lines is the wrong abstraction: split by concept (M8), never by trimming comments or lines. | review |

## The test-file mapping (decision)

```
crates/transport/src/host_token.rs        the code
crates/transport/src/host_token_tests.rs  all unit tests for host_token.rs
crates/transport/tests/transport.rs       public-API tests of the crate entry (src/transport.rs, or the crate name)
crates/transport/tests/host_token.rs      public-API tests aimed at host_token.rs, only if it is part of the public API
crates/transport/tests/support/*.rs       shared fakes and fixtures for tests/
crates/transport/src/test_support/*.rs    shared fakes for unit tests (declared once under #[cfg(test)] in lib.rs)
```

`host_token.rs` ends with exactly:

```rust
#[cfg(test)]
#[path = "host_token_tests.rs"]
mod tests;
```

Why this shape: the test file sits beside the code (the owner's "beside" rule, like Go's `foo_test.go`); `#[path]` keeps the modern `foo.rs` + `foo/` layout without `foo/tests.rs` hiding it in a folder, and the child module can reach private items of its parent, so unit tests stay unit tests. `mod.rs` is banned (lint `mod_module_files`).

Exceptions, all enforced by the same script (nothing else is exempt):

- `lib.rs`: declaration-only (`mod`, `pub use`, `//!`); the script rejects any `fn`/`impl`/type in it, so it needs no test file. Its public surface is covered by `tests/<crate>.rs`.
- `main.rs`, `build.rs`, `src/bin/*.rs`: thin entry points, no counterpart required (any logic they hold moves to a module).
- A source file with no `fn` at all (pure type or constants file) needs no test file.
- `tests/support/` and `src/test_support/`: shared fakes, not test files; they hold no `#[test]`.
- An integration test `tests/x.rs` must have `src/x.rs` or `x` must be the crate directory name.
- Source files may hold no `#[test]`; test files hold no non-test code.

## Public surface

`unreachable_pub = deny` makes `pub` mean "reachable from the crate root". A `pub` item that cannot be reached must be `pub(crate)`. `lib.rs` re-exports the entry, `Error`, traits and the types that appear in their signatures (M14); all `mod` declarations are private. Public types have private fields (C-STRUCT-PRIVATE), implement `Debug` (`missing_debug_implementations`), and public enums that may grow are `#[non_exhaustive]`. Public items of the shared crates are documented; `pub(crate)` and private items are not.

## Lint table for `app/Cargo.toml`

Owner of `app/Cargo.toml` pastes this verbatim; every member crate then has `[lints] workspace = true`. Thresholds live in `app/clippy.toml`. An exception is a crate-level `#![allow(lint, reason = "...")]` or `#[expect]` with a reason (script `allow`), e.g. `unsafe_code` in `webview`.

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
```

CI wiring belongs to the CI agent: run `--full` on the Linux and macOS runners.

## Notes

- `redundant_pub_crate` and `missing_const_for_fn` are not enabled: the first conflicts with `unreachable_pub`, the second is noise.
- `pub_use`, `single_call_fn`, `multiple_inherent_impl` stay off: re-exports are the API mechanism (M14).
- `excessive_nesting` counts nested blocks inside a function; verify the threshold in `app/clippy.toml` against M2 on the first compile and adjust the number, never the rule.
