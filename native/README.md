# `native/` — the Rust-native GPUI port

Spec: [`docs/superpowers/specs/2026-07-30-rust-native-desktop-port-design.md`](../docs/superpowers/specs/2026-07-30-rust-native-desktop-port-design.md).
Live state and the work queue: [`QUEUE.md`](QUEUE.md) — read it before starting
anything.

This is a **second** desktop client for the same Go daemon. `api/`, `web/` and
`desktop/` are untouched by this workspace.

## The PATH gotcha

`cargo` and `rustc` are installed but are **not on the default `PATH`**. Every
shell needs:

```sh
export PATH="$HOME/.cargo/bin:$PATH"
```

If `cargo: command not found` is the first thing you see, that is why. It is not
a missing toolchain.

## Build

```sh
cd native
cargo build --workspace
```

`rust-toolchain.toml` pins 1.96.0 with `rustfmt`, `clippy` and
`llvm-tools-preview`, and the two macOS targets for the universal binary. rustup
installs it on first invocation.

## Checks

All four must pass, with zero warnings, before an item is done:

```sh
cd native
cargo build --workspace
cargo clippy --workspace --all-targets -- -D warnings
cargo test --workspace
./scripts/check-invariants.sh
```

`check-invariants.sh` enforces the §4.3 rules that live outside the type system.
Coverage (`cargo llvm-cov --fail-under-lines 98` on the logic crates) is a
separate §12 gate and needs `cargo-llvm-cov`, which is not installed yet.

## The two rules a newcomer will otherwise break

**1. `crowbar-core` has no `gpui`. Ever.** It is the crate that holds all the
domain logic — git model, diff algebra, keymap resolution, settings validation,
file-tree model, workspace scoping, review threads — and it is held to ≥98% line
coverage precisely because none of it needs a window to test. If a piece of code
here seems to want `gpui`, it is not logic: split the rendering half out into
`crowbar-ui` or `crowbar-state` and leave the decision behind.
`scripts/check-invariants.sh` fails on the manifest and on `gpui::` in the
sources.

**2. Design tokens are sealed newtypes, so colour and spacing literals will not
compile at a call site.** `Color`, `Space`, `Radius`, `FontSize` and `Duration`
live in `crowbar-ui` with private inner fields and a `pub(crate)` constructor —
no `from_raw`, no public `new`. Write `theme.surface.raised`. You cannot write
`rgb(0x1e1e1e)` outside `crowbar-ui`, and that is deliberate: it moves
"an agent hardcoded an offset to force the parity oracle to converge" from
something a reviewer has to catch into something the compiler rejects.

Two more that cost less to learn early:

- **`crowbar-platform` is the only crate allowed to write `unsafe`.** Everything
  else carries `#![forbid(unsafe_code)]`, which is a hard compile error, not a
  lint. Platform code needs a `# Safety` doc comment on the enclosing item of
  every `unsafe` construct, and the check script fails without one.
- **`unwrap`, `expect`, `panic!`, `todo!` and `unimplemented!` are denied
  outside tests**, along with all of `clippy::pedantic`. Tests are exempt
  centrally via `clippy.toml`; never add a per-file `#[allow]`, because that is
  indistinguishable from silencing the lint in shipping code.

## Layout

```
native/
├── Cargo.toml            workspace root · workspace lints (§4.3 rule 4)
├── rust-toolchain.toml   pinned 1.96.0
├── clippy.toml           the test exemptions, centrally
├── QUEUE.md              done / in-flight / blocked · how a cold session resumes
├── crates/
│   ├── crowbar-proto/    serde DTOs, generated from the Go handlers
│   ├── crowbar-client/   unix-socket HTTP + WS, reconnect, backoff
│   ├── crowbar-core/     ALL domain logic · no gpui, ever
│   ├── crowbar-ui/       design system · sealed token newtypes
│   ├── crowbar-state/    Entity<T> stores + event graph
│   ├── crowbar-terminal/ GPU text grid over the daemon VT model
│   ├── crowbar-editor/   editor integration, buffers, retained editors
│   ├── crowbar-diff/     native review surface
│   ├── crowbar-webview/  webview panes and windows
│   ├── crowbar-platform/ THE ONLY unsafe crate
│   ├── crowbar-driver/   extractor + injector + MCP · feature-gated
│   └── crowbar-app/      the binary
├── oracle/               the parity differ · corpus/ is append-only (§8.4)
├── scripts/              check-invariants.sh
├── vendor/               pinned gpui (item 0.2) — committed on purpose
└── tools/                protogen (item 0.5)
```

The dependency edges between these crates are not a suggestion; §4.2 fixes them
and the manifests are the enforcement. `crowbar-driver` and `crowbar-app` are
the only crates that may depend on everything.
