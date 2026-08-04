# Blocked on a React-side prerequisite — `repo-import-dialog` emits two `button` anchors

**Raised:** 2026-08-03, taking `repo-import-dialog`'s verdict.
**Fixed:** 2026-08-04, `native/p3.74-dialog-close`. **Not a port defect.**

## Fix landed

`web/src/components/ui/dialog.tsx`'s two `<DialogPrimitive.Close render={<Button
.../>}>` call sites (`DialogPopup` and `AppDialog`) now pass
`data-oracle-id="dialog-close"` on the `Button` element itself, the same
override shape `repo-icon-popover.tsx`'s own Buttons already used. The close
button no longer inherits `button.tsx`'s bare `"button"` default, so it can no
longer collide with an unnamed body `Button`.

**Blast radius, established before touching anything:**

* **Rust side: no changes required, anywhere.** `dialog.rs`, `alert_dialog.rs`,
  `repo_import_dialog.rs` and `detach_holder_modal.rs` (the only four Rust
  modules that compose this primitive) all call `.close_button(false)` on the
  vendor `GpuiDialog`, and none of them anchor a body `Button` either — every
  footer/body is rendered as one opaque, unanchored box. Grepped `"button"`
  (the literal id) across every file in `native/crates/` and `native/mapping/`:
  every hit belongs to the standalone `--surface button` control or to two
  unrelated components (`context_pill.rs`, `project_home_row.rs`) asserting an
  ARIA `role`, not an oracle id. None reference a dialog close. No
  `row_layout` test for any of the four surfaces asserts on `"button"` or
  `"dialog-close"` either — `dialog.rs`'s own
  `the_wrapped_popup_carries_every_contract_anchor` test checks for
  `dialog-popup`/`-header`/`-title`/`-footer` only.
* **`web/src/lib/oracle/extract.ts`'s `oracleSurfaceScope`: no changes
  required.** `dialog` and `alert-dialog` each declare **only their own root**
  (`anchors: ['dialog-popup']` / `['alert-dialog-popup']`) — the close button
  was never in either declared set, before or after this fix, so nothing there
  needed to name `"button"` and nothing needs to start naming
  `"dialog-close"`. `repo-import-dialog` and `detach-holder-modal` have **no**
  entry in that table at all (confirmed by grep), so both remain undeclared,
  whole-subtree captures — which is exactly the shape this fix unblocks.
* **A claim in the original diagnosis (directly below, kept for the trail)
  turned out not to match what the code and its tests currently do**: "the
  `dialog` surface itself... passes... its fixture has only the close button"
  reads as if `dialog`'s own automated capture includes the close button's
  anchor. It doesn't, on either side of this fix: `oracleSelectDeclaredAnchors`
  drops any anchor not in the declared list (verified by reading it, and
  independently by the passing `popover`/`select` scope tests in
  `extract.test.ts`, which exercise the identical code path against a fixture
  with three `button`-id children and confirm they are dropped, not kept), and
  `dialog::Dialog::render` never emits a close-button anchor on the captured
  side either. The historical "PASS 0 deltas over 4 anchors" verdicts recorded
  for `dialog` throughout `QUEUE.md` line up with a **hand-assembled**
  reference file (`QUEUE.md` ~line 6120: "`/tmp/p3-ref-dialog.json` was
  hand-assembled"), not with a capture taken through this tool — so nothing
  about this fix could have broken a passing `dialog` verdict, because no
  currently-reachable code path routes the close button into that surface's
  compared anchor set at all. Reported since a worker finding "nothing breaks"
  is still a finding, not silence.

**Regression coverage** (all mutation-verified — reverting the id override on
either call site was actually run and actually failed, then reverted back;
full failure text is in each file's own doc comment):

* `web/src/__tests__/components/ui/dialog.test.tsx` (new) — direct unit test
  of both call sites.
* `web/src/__tests__/components/layout/repo-import-dialog.test.tsx` — added
  one test against the real, reachable fixture this doc's own diagnosis used.
* `web/src/__tests__/components/layout/detach-holder-modal.test.tsx` — same,
  for the two-body-Button case.

No test asserted the old `button` id for a dialog close anywhere in the
existing suite (checked before writing the fix) — there was nothing to
un-assert.

**Gates, run in the foreground on `native/p3.74-dialog-close`:** `cargo clippy
--workspace --all-targets -- -D warnings` clean; `cargo test --workspace`
2271/2271 (matches trunk baseline exactly); `check-invariants.sh` 7/7; `cd web
&& bun run test` 364 files / 2718 tests, 0 failures (vitest's own `forks` pool
is flaky under this sandbox's concurrency at the default worker count —
`--maxWorkers=2` reproduces cleanly every time; not a defect in the change,
see the branch's own notes); `bun tsc --noEmit` clean.

---

## Original diagnosis (2026-08-03), preserved as written

## What happened

The surface drives fine and the reference captures fine. The **differ refuses
the reference**:

```
oracle: ref.json is not a v1 snapshot: `anchors[7].id`: anchor id `button`
appears twice; the differ matches by id and would have no way to say which of
the two it compared
```

The dialog renders two `<Button>`s — the "Import" submit and the header's
close — and neither names its anchor, so both inherit the primitive's default.
(They live in **two different files**; see the fix below, which is not where I
first said it was.)

## The general rule this exposes, which is worth more than the fix

`web/src/components/ui/button.tsx:69` sets `'data-oracle-id': 'button'` as the
component's **default**. So:

> **Any surface that renders more than one `Button` without overriding the id
> is uncapturable.** Not "captures badly" — the differ refuses outright.

That is the correct behaviour (matching by id is the whole contract), and the
refusal is exactly where you want it: at the reference, before a comparison
that could not mean anything. But it means the check belongs *before* a surface
is scheduled for a verdict, not at the end.

Surfaces already doing it right, because their call sites pass explicit ids:
`repo-icon-popover` (`-upload`, `-emoji`, `-github`). The same file's own
comment already records the precedent for namespacing —
`repo-import-dialog-popup` and `detach-holder-modal-popup` were namespaced in
P3.51 for exactly this reason, one level up. **The buttons were missed in that
pass.**

## ‼️ The fix is ONE line in the primitive, not two in this call site

**My first write-up of this file said to name "the two call sites in
`repo-import-dialog.tsx`". That was wrong — only *one* of the two buttons is in
that file.** `repo-import-dialog.tsx` has exactly one `<Button>` (the "Import"
submit, line 168). The second comes from the **`Dialog` primitive itself**:

```tsx
// web/src/components/ui/dialog.tsx:80-84
{showCloseButton && (
  <DialogPrimitive.Close
    aria-label="Close"
    render={<Button size="icon" variant="ghost" />}   // ← no data-oracle-id
```

It matches the reference exactly: one `button` at `(17, 515)` `414×28` is
Import, the other at `(407, 9)` `32×32` is the close X in the header corner.
A second copy of the same pattern is at `dialog.tsx:259` for the other dialog
variant.

**So the fix is one line in `ui/dialog.tsx` (twice), and it unblocks every
dialog surface at once**: give the close button `data-oracle-id="dialog-close"`.
Fixing it per-call-site would have been the wrong shape of change *and* would
have missed `detach-holder-modal`, the AddRepositoryModal, and every dialog
added later.

### The precise rule

A `Dialog` surface is uncapturable **iff its body contains at least one
un-named `Button`** — because the primitive's close button already occupies the
`button` id. That is why the `dialog` surface itself passes: its fixture has
only the close button, so there is nothing to collide with. One is fine; two is
a refusal.

`detach-holder-modal` has **two** un-named body Buttons, so it is affected and
will need the same fix before it can take a verdict.

## Method note: my static sweep was wrong in both directions

I tried to pre-screen every layout component by regex — count `<Button` tags
lacking `data-oracle-id` — to catch this class before spending verdicts. **Three
of eight rows contradicted measured reality**, and I only noticed because I
checked the table against surfaces whose verdicts I had already taken:

- it called `repo-icon-popover` (5 un-named) and `sidebar-project-header` (3)
  uncapturable — **both captured fine**
- it called `repo-import-dialog` fine (1) — **that is the one that failed**

Cause: `<Button\b[^>]*?>` stops at the first `>`, and `onClick={() => …}`
contains one, so the match was truncated before reaching the
`data-oracle-id` further down the multi-line tag. The fourth time a broken grep
has masqueraded as a finding in this port.

**Chasing why the sweep was wrong is what found the real fix.** The regex said
`repo-import-dialog` had one un-named Button and the reference showed two —
that contradiction is the whole diagnosis, and a sweep I had trusted would have
buried it.

## Cell note, unrelated but recorded here

The first attempt at this verdict was refused by the **driver**, not the
differ, and correctly: at the app's 1119px window, `h-[70vh]` makes the popup
783.3px, which needs 868px of window against a display granting 829px. The
driver declined rather than emit a snapshot whose every `visible` would be an
artefact of the window size. Resizing the app to 800px high (`70vh` = 560)
cleared it. **A `vh`-sized surface's cell therefore depends on the display**,
which is worth knowing before blaming a port.
