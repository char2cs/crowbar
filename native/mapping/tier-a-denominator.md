# Tier A denominator — `crowbar-core`, measured from the React source

Companion to QUEUE.md's "The real Tier B denominator" — same rigour, different
target. Spec §16 Phase 3: *"Tier A (`core`, `proto`, `client`, theme tokens —
gated by ported tests) and Tier B."* `crowbar-core`'s `Cargo.toml` claims **"all
Crowbar domain logic: git model, diff algebra, keymap resolution, settings
schema, file-tree model, workspace scoping, review threads."** Today it is
`color.rs` + `lib.rs`, 349 lines, 100% coverage over a crate that holds none of
that domain logic (QUEUE.md, 2026-08-03).

**Status: IN PROGRESS. This skeleton is committed first per the interruption
protocol; each area below is filled in and committed as it completes.**

Method, per area: (1) where the logic lives today in `web/src`, with line
counts: (2) what if anything is already ported into `crowbar-proto` /
`crowbar-client`; (3) whether it is expressible with zero reference to a view,
store or framework (gpui-free, D2); (4) the bucket — Tier A core / Phase 4
state / already done / presentation / out of scope; (5) existing test files
and case counts, since §16 gates Tier A on ported tests.

---

## 1. Git model

*(pending)*

## 2. Diff algebra

*(pending)*

## 3. Keymap resolution

*(pending)*

## 4. Settings schema

*(pending)*

## 5. File-tree model

*(pending)*

## 6. Workspace scoping

*(pending)*

## 7. Review threads

*(pending)*

---

## Theme tokens (also named in §16 Phase 3 Tier A, alongside `core`/`proto`/`client`)

*(pending — cross-check against §3.3's 274 measured `--` declarations and
whatever token-adjacent logic, if any, sits outside `styles/*.css`)*

---

## The headline denominator

*(pending — files / lines / test cases, split by bucket: Tier A core ·
Phase 4 state · already done (proto/client) · presentation · out of scope)*

## Findings — corrections to the brief

*(pending)*
