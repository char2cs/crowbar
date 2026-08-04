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
separate §12 gate.

> **One line of output is not ours and does not clear.** Cargo prints
> `the following packages contain code that will be rejected by a future version
> of Rust: block v0.1.6` on every build. `block` is a crates.io transitive
> dependency of the vendored `gpui` macOS stack (`static of uninhabited type`);
> `cargo report future-incompatibilities` shows it. Fixing it means either a
> `[patch]` or editing `vendor/`, both of which break the pin. Zero warnings
> means zero warnings *from the workspace* — clippy with `-D warnings` is clean.

## Run

```sh
cd native
CROWBAR_HOME=<the home the daemon was started with> cargo run -p crowbar-app
```

The binary derives the daemon's socket path from `CROWBAR_HOME` exactly the way
the Go daemon does (`crowbar-client`'s `socket` module), asks it
`GET /v0/health`, and shows the answer.

**The app does not start a daemon.** Start Crowbar-React (`make dev-desktop`)
first and leave it open — it owns the daemon — then run this against the same
`CROWBAR_HOME`. Starting a daemon by hand *first* and then launching the Tauri
app is a trap; `QUEUE.md`'s 0.4 section explains why.

If the daemon is down, the window says so. That is the designed outcome, not a
failure: a connection error is displayed, never panicked on.

## Side-by-side review capture (S0.6)

`scripts/capture-pair.swift` is the review instrument for the whole port from
here on — see
[`docs/superpowers/specs/2026-08-04-slice-based-port-method-design.md`](../docs/superpowers/specs/2026-08-04-slice-based-port-method-design.md)
§3.3. Per-component parity verdicts stopped being the gate; "the user has seen
it beside Crowbar-React and accepted it" is the gate, and this script produces
the two labelled PNGs that review sits on.

```sh
# 1. Launch both apps against the same CROWBAR_HOME, each already sized to
#    the same width/height by its own launch mechanism — this script does
#    not resize windows, see the file's own header comment for why.
CROWBAR_HOME=<home> make dev-desktop &     # Crowbar-React, owns the daemon
CROWBAR_HOME=<home> cargo run -p crowbar-app &

# 2. Find each one's pid — pid, not app name, is what disambiguates when a
#    sibling worktree also has a Crowbar window open (see the script's "WHY
#    PID, NOT JUST OWNER NAME").
pgrep -f "Crowbar.app/Contents/MacOS/Crowbar"   # or your dev binary's path
pgrep -f target/debug/crowbar-app

# 3. Capture the pair.
swift native/scripts/capture-pair.swift \
  --a-label react --a-pid <react-pid> \
  --b-label rust  --b-pid <rust-pid> \
  --width 1200 --height 800 \
  --out-dir native/capture-out
```

Two files land in `--out-dir`: `react-1200x800-<timestamp>.png` and
`rust-1200x800-<timestamp>.png`, plus a `pair-<timestamp>.json` manifest
recording each side's resolved window, its settled frame, and the blank-image
self-check's sampled/distinct pixel counts — the evidence that the capture was
taken from a settled, non-blank window, not just a filename claiming so.

**Known limit on the machine this was built on, read before assuming a run
failed silently:** pixel capture goes through `ScreenCaptureKit` (Apple
removed `CGWindowListCreateImage` in macOS 15; the script's header explains
what replaced it), which needs Screen Recording granted to whichever app is
the TCC-responsible ancestor of your shell — check with `ps -o
pid,ppid,comm= -p $$` up to the first `.app` bundle, then System Settings →
Privacy & Security → Screen Recording. The script preflights this and fails
fast with the exact remediation rather than writing a blank image. Pass
`--dry-run` to exercise window resolution, the settle step and the
forced-equal-size check without needing that permission at all.

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
├── scripts/              check-invariants.sh, capture-pair.swift (S0.6)
├── vendor/               pinned gpui (item 0.2) — committed on purpose,
│                         excluded from this workspace, never hand-edited
└── tools/                protogen (item 0.5)
```

The dependency edges between these crates are not a suggestion; §4.2 fixes them
and the manifests are the enforcement. `crowbar-driver` and `crowbar-app` are
the only crates that may depend on everything.

**`gpui` is a direct dependency of `crowbar-ui`, `crowbar-state` and
`crowbar-app` only.** The leaf view crates — `terminal`, `editor`, `diff`,
`webview` — reach it as `crowbar_ui::gpui` / `crowbar_ui::gpui_component`,
because §4.2 gives them `ui` and `state` and nothing else. That re-export is
deliberate: a framework bump, or a wrapper interposed in front of a
`gpui-component` type, is then one edit in the design system rather than one per
crate. See `vendor/PINNED.md` for the pins themselves.
