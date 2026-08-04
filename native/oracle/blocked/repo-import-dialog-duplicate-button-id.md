# Blocked on a React-side prerequisite — `repo-import-dialog` emits two `button` anchors

**Raised:** 2026-08-03, taking `repo-import-dialog`'s verdict.
**Blocked on:** a one-line-per-call-site change in `web/src`, the same shape as
P3.51's and P3.54's prerequisites. **Not a port defect.**

## What happened

The surface drives fine and the reference captures fine. The **differ refuses
the reference**:

```
oracle: ref.json is not a v1 snapshot: `anchors[7].id`: anchor id `button`
appears twice; the differ matches by id and would have no way to say which of
the two it compared
```

The dialog renders two `<Button>`s — the "Import" submit and the header's
close — and **neither call site names its anchor**, so both inherit the
primitive's default.

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

## The fix

Give the two call sites explicit ids in `repo-import-dialog.tsx`, matching the
port's own constants:

- the submit button → `repo-import-dialog-submit`
- the header close button → `repo-import-dialog-close`

Then re-run the verdict. Nothing in the Rust port changes unless the port has
no anchors for them, in which case it gains two.

## Also check before scheduling

`detach-holder-modal` is the other ⏸ standalone modal and comes from the same
P3.51 cluster. It very likely has the same duplicate-`button` problem; check
its call site in the same pass rather than discovering it one verdict later.

## Cell note, unrelated but recorded here

The first attempt at this verdict was refused by the **driver**, not the
differ, and correctly: at the app's 1119px window, `h-[70vh]` makes the popup
783.3px, which needs 868px of window against a display granting 829px. The
driver declined rather than emit a snapshot whose every `visible` would be an
artefact of the window size. Resizing the app to 800px high (`70vh` = 560)
cleared it. **A `vh`-sized surface's cell therefore depends on the display**,
which is worth knowing before blaming a port.
