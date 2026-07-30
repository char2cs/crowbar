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

---

## Done

_(none)_

## In flight

_(none)_

## Blocked — needs a user decision

_(none)_

---

## Phase 0 items

Owner column: `W` = dispatched worker, `O` = orchestrator-only.

| # | Item | Spec | Owner | Status |
|---|---|---|---|---|
| 0.1 | `native/` workspace scaffold, 14 crates per §4.2 with the §4.3 compiler-enforced rules | §4.2 §4.3 §4.4 | W | todo |
| 0.2 | Vendor + pin `gpui` at a SHA under `native/vendor/gpui/` | §10.5 | W | todo |
| 0.3 | `gpui` + `gpui-component` skills into `.claude/skills/` | §16 | W | todo |
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
