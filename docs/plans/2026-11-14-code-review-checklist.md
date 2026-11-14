# Code review checklist (per crate)

Used by the reviewer agent for every Rust crate and the Swift package before the gate. Rules referenced as M# are in `docs/rust-style.md` and `docs/swift-style.md`. A crate passes only when every item is yes, or a finding is filed with file:line and a concrete fix. Start by running `tools/scripts/check-rust-rules.sh --full` (Swift: `check-swift-rules.sh --full`); a red run is a failed review before reading any code.

Output per crate: verdict (pass / fail), findings table (severity, file:line, rule, fix), and the list of items checked.

## 1. Error handling (M9, M26, M27)
- Every fallible fn returns `Result<_, Error>`; `Error` is a `thiserror` enum with sources and context; no `anyhow` in libraries.
- No `unwrap`, `expect`, `panic!`, `todo!`, indexing or slicing that can panic, integer overflow in release paths, outside tests and `main.rs`.
- No swallowed errors: `let _ =`, `.ok()`, `unwrap_or_default()` on a failure the caller needs; logged-and-continued errors state why that is safe.
- Errors cross layers with context saying what was attempted; user-visible messages carry no secrets or tokens.

## 2. Unsafe audit
- `unsafe_code` is denied crate-wide; a crate that allows it names the reason and is limited to FFI/platform files.
- Each `unsafe` block is one operation, has a `// SAFETY:` comment stating the invariant and who upholds it, and is wrapped in a safe API that cannot be misused.
- Raw pointers, `transmute`, `from_raw_parts`, `Send`/`Sync` impls are justified; `unsafe impl` has a test or proof sketch; FFI strings and lifetimes are checked (no use after free, no dangling `CString`).
- Every `Drop` of an FFI resource is correct on panic and on early return.

## 3. Concurrency and cancellation (M25)
- No lock held across `.await`; no blocking call (file, sleep, mutex wait, DNS) on an async task or on the GPUI main thread.
- Every spawned task has an owner that joins or aborts it; no detached `spawn` whose failure vanishes.
- Cancellation is explicit (token or drop-safe) and tested: cancelling mid-operation leaves no half-written state.
- Channels are bounded or the bound is argued; back-pressure and slow consumers are handled; no `unbounded` growth.
- Lock order is documented where two locks are taken; shared state has one owner (no second copy to resync).
- UI-thread-only types never cross threads; `Send`/`Sync` bounds are not loosened to compile.

## 4. Resource leaks and shutdown
- Sockets, child processes, PTYs, webviews, file handles, timers, subscriptions are closed on every path (RAII or explicit `shutdown`).
- Caches, pools and history are bounded with an eviction rule; the soak target is bounded memory.
- Clean shutdown: tasks stop in order, children are reaped, no zombie daemon; shutdown is idempotent.
- Reconnect and daemon-recovery loops back off, have a ceiling, and do not leak the previous attempt.

## 5. API surface (M13-M17)
- `lib.rs` is declaration-only and re-exports only the entry, `Error`, traits and types in their signatures; `unreachable_pub` is clean.
- Public structs have private fields; open enums are `#[non_exhaustive]` where they may grow; no leaking of dependency types in signatures unless intended (C-STABLE).
- Common traits implemented (`Debug`, `Clone`, `PartialEq`, `Default`, `Send`, `Sync`) where meaningful (C-COMMON-TRAITS, C-SEND-SYNC).
- Shared crates do not depend on `gpui`/`tauri`/`wry`; dependency direction is one way; no peer imports another peer's internals.
- No stateless struct that should be a function; `new` only with real state; no bool parameters.

## 6. Docs (M7)
- Every `pub` item has a doc comment with `# Errors`, `# Panics`, `# Safety` where applicable; crate-level `//!` states purpose and entry.
- No doc comments on private items, no narrative `//` comments, only `SAFETY:`/`SPEC:`/`PLATFORM:`/`DEPRECATED:`; no incident narratives or PR history.
- Intra-doc links resolve (`cargo doc --no-deps` clean with `-D warnings`).

## 7. Tests (M39-M43)
- Every source file with a function has `<file>_tests.rs`; no free tests, no inline `mod tests {}`.
- Tests are table-driven and assert behaviour through the public interface; errors paths and cancellation are covered, not only the happy path.
- No real network, daemon, display or home directory by default; no `sleep`; deterministic clocks and channels.
- No test written only for coverage; no `#[ignore]` without a reason and a CI job that runs it.
- Bug fixes come with the test that failed first.

## 8. Performance
- No allocation or clone in per-frame, per-keystroke or per-byte paths without a measurement; `&str`/slices over `String`/`Vec` at boundaries.
- No O(n^2) over panes, tabs, messages or terminal cells; large collections are indexed or virtualised.
- Render and layout code does no I/O and no lock waits; work is moved off the UI thread and results are applied in one place.
- Hot paths (terminal feed, frame decode, transport) have a benchmark or a recorded measurement; regressions are explained.
- Startup does only what the first window needs.

## 9. Naming and structure (M8, M21, M22, M29-M33)
- One type per file named after it; no `util`/`common`/`helpers`/`base`/`misc`; no stutter (`transport::Client`).
- Names reveal intent; no magic numbers or strings; functions under 40 lines and 2 nesting levels; guard clauses, no `else` after return.
- Files under 500 lines; a file near the limit is split by concept now.
- No duplicated logic seen three times; no dead code, no `#[allow(dead_code)]`, no unused dependency (`cargo machete`).

## 10. Clippy pedantic and tooling (M1-M6)
- `cargo fmt --check`, `cargo clippy --workspace --all-targets -- -D warnings` (all + pedantic + the table in `docs/rust-style.md`) are clean without new `allow`s; every `allow`/`expect` names the lint and a reason.
- No loosened lint, threshold or baseline to get green.
- `cargo deny check` clean: licenses (permissive only), advisories, bans, sources.

## 11. Security
- Inputs from the daemon, web views, extensions, the clipboard and the file system are validated at the boundary; sizes are capped (pane `state` 64 KB, frame sizes, line lengths); no unbounded reads.
- TLS pinning and host tokens: constant-time compares where relevant, tokens never logged or put in URLs, stored only in the OS keychain, zeroized or short-lived in memory.
- Web view navigation policy is allow-list based; `crowbar://` handlers canonicalise paths and reject traversal; IPC messages are origin- and identity-checked per webview.
- Child processes are spawned without a shell, with an explicit environment and no user-controlled arguments reaching a shell; file permissions on sockets and token files are restrictive.
- No secrets, tokens or home paths in logs, panics, error strings or test fixtures; dependencies are pinned in `Cargo.lock` and pass `cargo deny`.
- Providers' real home directories and prod are never touched by tests.

## 12. Platform
- No Windows code paths (not supported). Linux (XWayland) and macOS differences sit behind one platform module with `PLATFORM:` comments for each constraint.
- The iOS plugin keeps FFI minimal; Swift side follows `docs/swift-style.md`.
