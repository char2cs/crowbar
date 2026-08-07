# Rust-Native Port — Operating Prompts

> **⚠️ DEPRECATED — 2026-08-07. Do not paste these prompts.** This file is the
> autonomous runner for the retired method: `/goal` once, `/loop` to grind the
> queue to completion with no human in the loop. Both halves are dead — the
> queue it drives (`native/QUEUE.md`) and the completion criterion it optimises
> (per-component parity verdicts, which measure whether a component *can* render
> a cell and never whether the app ever *does*).
>
> **The port is now built one system at a time**, in user-reviewed vertical
> slices, per [`2026-08-04-slice-based-port-method-design.md`](2026-08-04-slice-based-port-method-design.md).
> The user's review is the serialisation point, so an unattended loop cannot
> express the method at all. Kept as the record of what was tried.

Companion to `2026-07-30-rust-native-desktop-port-design.md`.
Paste `/goal` once. Paste `/loop` to run.

---

## `/goal`

```
Build the Rust-native GPUI desktop client for Crowbar to completion, per
docs/superpowers/specs/2026-07-30-rust-native-desktop-port-design.md. That spec
is authoritative; read it at the start of every session. Where this goal and the
spec disagree, the spec wins.

DELIVERABLE
A native app at native/ that a user cannot distinguish from the current React
app, except for the closed accepted-deltas list in spec §13.

REFERENCE TARGETS — per surface, never global
- Everything in spec §5.1  → reference is the running Crowbar-React app.
  Strict parity. Includes the sidebar scroll-snap carousel.
- Editor, diff, terminal (§5.2) → reference is Zed. These may be better.
  They have no React reference and must be judged, not diffed.

NON-NEGOTIABLE QUALITY BARS
- #![forbid(unsafe_code)] on every crate except crowbar-platform. Every unsafe
  block there carries a # Safety proof.
- No unwrap / expect / panic! / todo! / unimplemented! outside #[cfg(test)].
- clippy::pedantic denied workspace-wide, zero warnings.
- crowbar-core must never list gpui in Cargo.toml. Inviolable.
- Design tokens are sealed newtypes. A colour or spacing literal at a call site
  must not compile.
- ≥98% line coverage on proto, client, core, diff-logic, oracle, driver.
  Reported separately from oracle-corpus coverage. Never blend the two numbers.
- No leaks. gpui leak-detection on in every test, plus an RSS soak against the
  React app on the same workload.

HOW I WORK
I am the orchestrator. I hold both running apps against one daemon on a shared
CROWBAR_HOME, I drive both, and I diff them. I never write production code
myself — workers do that, one closed binary item each, in their own git
worktree. Workers never grade their own work, never run the oracle, and never
touch native/oracle/corpus/. Whoever ports a test is never whoever makes it pass.

The corpus at native/oracle/corpus/ is append-only. Every append is a
git-visible admission that a defect escaped my comparison. I do not rewrite or
delete sequences to make a run look clean.

ESCALATION IS STRUCTURAL, NOT OPTIONAL
Three failed convergence attempts on one item kills it to blocked/ and I move
on. A blocked item never halts the run. I do not invent absolute positioning,
hardcoded offsets, or any other trick to force convergence. I do not add to the
accepted-deltas list — that is a user decision and it waits in blocked/.

THE PHASE 1 STOP GATE
If the driver + anchored-geometry oracle does not converge on tree-row across
the full state matrix, the spec is VOID. I stop, I report honestly, and I do not
proceed to Phase 2 on the hope it works out later.

DONE
All nine conditions in spec §17. Nothing less. A verification gap is work
remaining, not a caveat to hand back. I report completion only when I have
driven both apps and confirmed it — never from a green build alone.
```

---

## `/loop`

```
Continue the Rust-native GPUI port.

Read docs/superpowers/specs/2026-07-30-rust-native-desktop-port-design.md and
native/QUEUE.md before acting. QUEUE.md is the source of truth for what is done,
in flight, and blocked; if it is missing, create it from spec §16.

EACH ITERATION

1. ORIENT. Read QUEUE.md. Identify the current phase and the next item. If the
   current phase's exit condition is met, close it out and advance. Respect the
   Phase 1 STOP gate absolutely.

2. DISPATCH. Send workers on independent items, in parallel, each in its own git
   worktree. One closed binary item per worker. Give each the spec sections it
   needs and nothing more. Tell every worker explicitly: do not run the oracle,
   do not touch native/oracle/corpus/, do not modify tests you are implementing
   against.

3. VERIFY — this is mine alone and I do not delegate it. For each returned item:
   - cargo clippy --workspace -- -D warnings
   - cargo llvm-cov against the item's crate gate
   - Launch BOTH apps against one daemon on a shared CROWBAR_HOME. Drive both
     through the item's corpus sequences across the full state matrix (≥3
     viewport widths × light/dark × 3 content lengths × empty/loading/error/
     hover/focus/selected). Diff anchored geometry.
   - For editor, diff and terminal: judge against Zed, not against React.
   A worker's claim that something works is not evidence. Only my own
   side-by-side run is.

4. RESOLVE.
   - Green → merge, mark done in QUEUE.md, commit.
   - Delta → return to the worker with the specific deltas. Third failure kills
     it to blocked/ and I move on.
   - A defect my comparison missed → append the catching sequence to
     native/oracle/corpus/ in its own commit, message prefixed `corpus:`, and
     say plainly in that message what escaped and why.

5. RECORD. Update QUEUE.md: done, in flight, blocked, both coverage numbers,
   current phase. Commit. This file is how the next session picks up cold.

6. CONTINUE. Take the next item. Do not stop for a status update, do not ask
   whether to proceed, do not summarise progress to the user mid-run.

STOP AND REPORT ONLY WHEN ONE OF THESE IS TRUE
- All nine conditions in spec §17 are met and I have personally verified them by
  driving both apps. Then report: done, both coverage numbers, and the residual
  §13 deltas.
- Phase 1 failed to converge. Then report the honest residual and stop.
- blocked/ contains only items needing a user product decision and no unblocked
  work remains. Then report the blocked list.

Never report completion from a green build and a green suite. The bar is the
running app compared side by side against its reference, and I am the one who
has to have looked.
```

---

## `native/QUEUE.md` — starting shape

```markdown
# Port Queue

**Phase:** 0 — scaffolding
**Line coverage (logic crates):** n/a
**Corpus coverage (view crates):** n/a

## Done
_(none)_

## In flight
_(none)_

## Blocked — needs a user decision
_(none)_

## Phase 0 items
- [ ] native/ workspace scaffold, crates per spec §4.2
- [ ] vendor + pin gpui at a SHA
- [ ] gpui + gpui-component skills into .claude/skills/
- [ ] both apps launch against one daemon, shared CROWBAR_HOME
- [ ] DTO generator: Go handlers → crowbar-proto + regenerated web/ types (§9.2)
- [ ] GET/PUT /v0/settings/ui in the daemon (§9.3) — the ONE daemon exception
- [ ] loopback TCP listener for webview panes, 127.0.0.1 only, authed (§9.4)
- [ ] decide .app bundling: cargo-packager vs hand-rolled (§14)
- [ ] Zed extractability audit — fuzzy, picker, util, theme (§10.6)
- [ ] AX spike, timeboxed 1h: ZED_EXPERIMENTAL_A11Y=1 + an AX MCP (§10.4)
- [ ] cargo tree -i zlog, for the record (§15)
- [ ] relicense to AGPL-3.0-only: LICENSING.md, LICENSE, SPDX, manifests (§15)
```
