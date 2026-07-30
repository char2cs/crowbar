# Port Queue

Source of truth for the Rust-native GPUI port. Spec:
`docs/superpowers/specs/2026-07-30-rust-native-desktop-port-design.md`.
Updated every orchestrator iteration. This file is how a cold session picks up.

**Phase:** 0 — scaffolding and the cheap answers
**Line coverage (logic crates):** n/a — no logic crates exist yet
**Corpus coverage (view crates):** n/a — no oracle exists yet

---

## Environment facts (verified 2026-07-30)

Recorded because a cold session will otherwise rediscover them the hard way.

| Fact | Value |
|---|---|
| `rustc` / `cargo` | **1.96.0**, installed but **NOT on the default PATH**. Every shell needs `export PATH="$HOME/.cargo/bin:$PATH"`. |
| rustup toolchains | `stable` (active), `1.85`, `1.88` |
| Installed targets | `aarch64-apple-darwin`, `x86_64-apple-darwin`, `x86_64-pc-windows-msvc`, `x86_64-unknown-linux-gnu` |
| `cargo-llvm-cov` | **not installed** — required by §12. Install before the first coverage gate. |
| `cargo-nextest` | not installed |
| Go | 1.26.2 (`/opt/homebrew/bin/go`) |
| `bun` | `~/.bun/bin/bun`, also off the default PATH |
| Zed | `/Applications/Zed.app` present (stable channel) — used by the §10.4 AX spike |
| Network | reachable |
| Running daemon | a `crowbar-api` from the **`feature/crowbar-skill` worktree** is live on a temp socket. It is **not** this port's daemon; do not reuse it for parity runs. |

### The shared `CROWBAR_HOME` for parity runs (item 0.4)

`Makefile:11` scopes every `dev*` target to `CROWBAR_HOME ?= $(CURDIR)/.crowbar`.
Both apps inherit it, so "one daemon, shared home" is already the harness — the
native app only has to derive the same socket path.

| | |
|---|---|
| `CROWBAR_HOME` | `<worktree>/.crowbar` |
| fnv1a64 of it | `6d4f21ce150add3c` |
| Socket | `$TMPDIR/crowbar-6d4f21ce150add3c.sock` |
| Present? | no — no daemon has run against this worktree yet |

### Socket-path contract — `crowbar-client` must reproduce this exactly

`api/internal/core/gateway/transports/socket.go:117`. With `CROWBAR_HOME` set:

```
$TMPDIR/crowbar-<fnv1a64(CROWBAR_HOME)>.sock      // Go: fmt.Sprintf("crowbar-%x.sock", h)
```

with no `CROWBAR_HOME`: `~/.crowbar/crowbar.sock`.

> **TRAP — Go's `%x` does not zero-pad.** A hash with a leading zero nibble
> yields a *shorter* filename. There is a live example on this machine right
> now: `crowbar-83bf8eb85db086d.sock` is **15** hex chars, not 16. A Rust port
> written as `format!("crowbar-{h:016x}.sock")` would look correct, pass review,
> and then fail to find the daemon for roughly 1 workspace in 16 — surfacing as
> a bare connection error with nothing pointing at the cause. Use `{h:x}`.
>
> The socket deliberately does **not** live inside `CROWBAR_HOME`: macOS caps
> `sun_path` at 104 bytes and override homes under
> `~/.crowbar/projects/.../worktree` routinely exceed it. Do not "simplify" this.

`desktop/src-tauri/src/sidecar/mod.rs` already carries this fnv1a64 in Rust —
read it before writing a third copy.

---

## Done

### 0.3 — `gpui` + `gpui-component` skills vendored ✅

Merged `754e6c7b`. `.claude/skills/gpui/` (23 upstream files, 7,685 lines) and
`.claude/skills/gpui-component/` (3 files, 861 lines), from
`longbridge/gpui-component` pinned at `88f102d13654fe25aa2fede076274b6b751a3704`
(content revision `a183e4622b099968d9978796e647e6a75f0f5ac1`), Apache-2.0,
`LICENSE-APACHE` + `PROVENANCE.md` carried into each.

**Orchestrator verification** — not taken on the worker's word: `SKILL.md` for
both skills plus `gpui/references/entity-patterns.md` and
`gpui-component/references/usage.md` were re-fetched from
`raw.githubusercontent.com` at the pinned SHA and hashed. All four
byte-identical. Commit scope re-checked: nothing outside `.claude/skills/`.

> **Environment trap this surfaced:** this machine's `~/.gitignore_global` line 2
> is `.claude/`, so the entire vendor was silently unstageable until forced with
> `git add -f`. The repo's own `.gitignore` has no such rule, so CI and fresh
> clones are unaffected — but **anything new added under `.claude/` on this
> machine needs `-f`**, and a worker who does not notice will report success
> having committed nothing.

Not vendored: upstream's third skill `gpui-component-dev` — it is a contributor
workflow for adding components *to* `gpui-component`, which we are consuming, not
contributing to.

## In flight

Wave 1 — dispatched 2026-07-30. Ten workers, disjoint owned paths, one worktree
each. Every brief carries the worker contract: do not grade your own work, do
not run the oracle, do not touch `native/oracle/corpus/`, do not modify tests
you are implementing against.

| Item | Branch | Owned paths |
|---|---|---|
| 0.1 scaffold | `native/0.1-scaffold` | `native/{Cargo.toml,rust-toolchain.toml,clippy.toml,crates/**,scripts/**}`, `native/oracle/{Cargo.toml,src/**}` |
| 0.2 vendor gpui | `native/0.2-vendor-gpui` | `native/vendor/**` |
| 0.3 skills | `native/0.3-skills` | `.claude/skills/{gpui,gpui-component}/**` |
| 0.5 protogen | `native/0.5-protogen` | `native/tools/protogen/**` |
| 0.6 settings/ui | `native/0.6-settings-ui` | `api/**` |
| 0.7 loopback TCP | `native/0.7-loopback-tcp` | `api/**` (serve wiring) |
| 0.12 relicense | `native/0.12-agpl-only` | everything except `native/` |
| 0.8 bundling | — read-only research | — |
| 0.9 Zed audit | — read-only research | — |
| 0.10 AX spike | — read-only research, 1h timebox | — |

0.6 and 0.7 both live in `api/`; they are merged serially, 0.6 first.

## Blocked — needs a user decision

_(none)_

---

## Phase 0 items

Owner column: `W` = dispatched worker, `O` = orchestrator-only.

| # | Item | Spec | Owner | Status |
|---|---|---|---|---|
| 0.1 | `native/` workspace scaffold, 14 crates per §4.2 with the §4.3 compiler-enforced rules | §4.2 §4.3 §4.4 | W | todo |
| 0.2 | Vendor + pin `gpui` at a SHA under `native/vendor/gpui/` | §10.5 | W | todo |
| 0.3 | `gpui` + `gpui-component` skills into `.claude/skills/` | §16 | W | **done** |
| 0.4 | Both apps launch against one daemon on a shared `CROWBAR_HOME` | §0 §9.1 | O | todo — gated on 0.1 + 0.2 |
| 0.5 | DTO generator: Go handlers → `crowbar-proto` + regenerated `web/` types | §9.2 | W | todo |
| 0.6 | `GET`/`PUT` `/v0/settings/ui` in the daemon — the ONE daemon exception | §9.3 | W | todo |
| 0.7 | Loopback TCP listener for webview panes, `127.0.0.1` only, authed | §9.4 | W | todo |
| 0.8 | Decide `.app` bundling: `cargo-packager` vs hand-rolled | §14 | W | todo |
| 0.9 | Zed extractability audit — `fuzzy`, `picker`, `util`, `theme` | §10.6 | W | todo |
| 0.10 | AX spike, timeboxed 1h: `ZED_EXPERIMENTAL_A11Y=1` + an AX tree dump | §10.4 | W | todo |
| 0.11 | `cargo tree -i zlog`, for the record | §15 | O | todo — gated on 0.2 |
| 0.12 | Relicense to AGPL-3.0-only: `LICENSING.md`, `LICENSE`, SPDX, manifests | §15 | W | todo |

**Phase 0 exit condition:** every row above is `done` or written to
`native/oracle/blocked/`, and `cargo clippy --workspace -- -D warnings` is clean
from `native/`.

---

## Phase 1 — THE GATE (not started)

Build `crowbar-driver` (extract + inject + MCP over stdio) and the
anchored-geometry differ. Prove convergence on `components/ui/tree-row.tsx`
across the full §8.3 matrix.

> **STOP.** If Phase 1 does not converge, the spec is void. Report honestly and
> do not proceed to Phase 2. The implementation plan is written only after
> Phase 1 reports.

---

## Ledger

Append-only. One line per orchestrator iteration.

- `2026-07-30` — Queue created from spec §16. Toolchain blocker resolved
  (`cargo` was installed, just off PATH). Phase 0 wave 1 dispatched.
