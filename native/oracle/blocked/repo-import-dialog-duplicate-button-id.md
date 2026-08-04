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
