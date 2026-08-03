# §13 accepted delta — native menus, DECIDED BY THE USER

> **✅ RESOLVED 2026-08-03 — nothing here is blocked any more.** The decision was
> the user's and was made on 2026-07-31; the spec edit it implied was the only
> thing outstanding, and it landed in `a4de9aef` (spec §13 entry 7, plus the §5.1
> carve-out that phrase needed). The file stays at this path because the rest of
> the project cites it, and because the reasoning — what the delta costs, what it
> buys, and the four things `NSMenu` cannot express — is worth keeping. Under
> §17.7 it is a **listed user decision**, not an open blocker.

**Decided:** 2026-07-31, verbatim: *"Dropdown menus should be native, not
'react' simulated."*
**Status:** decided. What waits here is the **spec edit**, not the decision.

This file exists because §13's accepted-deltas list is a user decision and I do
not add to it on my own. The user has now made one, so this records it and the
formalisation it implies, rather than my having quietly edited the spec.

## The delta

Crowbar-native uses **real platform menus**, so a menu will not match the React
app pixel-for-pixel. It will look like a macOS menu, because it *is* one.

Concretely, against `web/src/components/ui/dropdown-menu.tsx`: the popup's
background, radius, border, padding, item height, font and the highlight colour
are all AppKit's, not `theme.css`'s. Keyboard navigation, Escape,
click-outside, submenu open/close timing and screen-edge flipping become the
OS's behaviour rather than base-ui's.

## What it costs, stated plainly

`dropdown-menu` leaves the §5.1 strict-parity gate. It **cannot** be
anchor-diffed: an `NSMenu` is not in the window's view tree and has no
`data-oracle-id` to read. So this surface is **judged, not diffed** — the same
treatment §5.2 gives the editor, diff and terminal, and for the same reason:
there is no comparable reference.

That is a real reduction in what the oracle covers, and it should be visible in
the final report rather than absorbed silently.

## What it buys

OS keyboard navigation, VoiceOver reachability, submenu timing and screen-edge
flipping, none of which we would otherwise owe an implementation — §10.4 dropped
the AX spike as "THIN", and a platform menu is accessible without it.

## Consequences already handled

- **P2.1's GPUI port is superseded, not deleted.** It reproduced base-ui's
  structure and Tailwind styling specifically so it could be pixel-diffed. Its
  `MAPPING.md` section stays — the `ring-1`-is-a-box-shadow trap and the measured
  Tailwind values are surface-independent. The component is retired only once the
  native one is proven.
- **P2.8's fixture is not wasted.** The review thread it created through the
  daemon's API is what makes any menu reachable at all, and a native menu still
  needs a trigger to judge.
- **P2.9, the parity capture, was stopped mid-flight** rather than finished, as
  soon as the target changed. Its partial work is discarded on purpose.

## The formalisation still owed

Spec §13's list should gain an entry naming this delta, and §5.1 should stop
listing `dropdown-menu` as strict-parity. **I have not edited the spec** — it is
authoritative, and a §13 entry is exactly the thing I am told not to add on my
own initiative. The decision is made; the edit is a separate, deliberate act.

## The one open implementation question

`NSMenu` for true context menus, **GPUI-drawn** for anything that must carry the
design tokens or live inside a pane. The user did not rule between them, so I
took that split as the reading and said so before acting. If it is wrong, the
`NSMenu` work is confined to `crowbar-platform` and a driver surface, and the
GPUI path is what P2.1 already built.


---

## P2.14 landed — and it duplicates something already vendored. My error.

`crates/crowbar-platform/src/native_menu.rs` is merged, green and genuinely
good: 755 tests, clippy clean, 7 `ok` lines, Phase 1's two archived snapshots
re-emit byte-identical, and **rule 3 bites** — removing a single `# Safety`
heading fails the build, which I verified by mutation myself. It is the first
`unsafe` this project has shipped and the proofs are real arguments, not
gestures.

**But `vendor/gpui-component/src/native_menu/` already does this job** — 1,252
lines across `macos.rs`, `windows.rs` and `fallback.rs`, gpui-integrated
(`show(position, window, cx)`), with icons and `Box<dyn Action>` dispatch.

**That is my mistake, not the worker's.** I briefed P2.14 without checking
whether the capability already existed in a dependency we already ship. The
worker built exactly what was asked and then told me, which is the right
behaviour and the reason I know.

### Ruling

**Call sites use the vendored menu.** It takes a gpui `Point<Pixels>`, dispatches
`Action`s, and is what an idiomatic gpui call site wants; it also already covers
Windows, which ours does not. Blocking for a return value, as ours does, is not
how a gpui view is written.

**`crowbar-platform::native_menu` is retained for now and retired before Phase 3
closes** unless a concrete need appears that the vendored one cannot serve. It is
wired to **no** call site — only to its own driver surface — so it ships nothing
today and costs only its own maintenance.

**P3.40 retired the implementation.** No such need appeared — P3.38
confirmed that every call site exercised needs the vendored menu already
serves. See commit `bca7c3c9`.

Two things it holds that the vendored one does not, and which decide the question
if either turns out to matter:

- it is **gpui-free**, which §4.2 requires of `crowbar-platform` and which makes
  it testable with no window and no `App` — worth something on a machine where
  synthetic events are denied and 15 of its 16 checklist items need a human;
- it returns the **chosen id synchronously**, which a test can assert on.

### Findings from P2.14 worth keeping regardless of which one ships

- **`NSMenu` cannot express four things Crowbar's menus use**:
  `menu-radio-item`, `menu-label`, `menu-shortcut` and `inset`. A radio group
  becomes ticked items whose exclusivity the *caller* owns; a label becomes a
  disabled item; a shortcut becomes the responder chain's business. **Any call
  site depending on those, or rendering arbitrary content in a row, cannot move
  to a native menu** and must stay on P2.1's GPUI popup. That has to be decided
  per call site before P2.1 is retired — it is not one global switch.
- **A GPUI-scheduled dismissal silently does not work.** GPUI's foreground
  executor schedules onto the main dispatch queue, and libdispatch will not begin
  another main-queue block while one is on the stack — so a timer armed that way
  fires only *after* the menu closes by itself. An `NSTimer` in
  `NSRunLoopCommonModes` is the way.
- **`--test-threads=1` is a live hazard**: libtest runs on the process main
  thread at concurrency 1, so a test calling `show_at` unconditionally would pop
  a real menu and hang the suite.
- Rule 3 has a blind spot it cannot see: `define_class!`'s `#[unsafe(super(…))]`
  and `#[unsafe(method(…))]` are unsafe *attributes*, not blocks, so the scanner
  skips them. Documented in the module rather than left to be discovered.
