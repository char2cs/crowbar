# `alert-dialog` (P3.28) — a variant of `dialog`, proven by the class diff

`web/src/components/ui/alert-dialog.tsx` →
`crates/crowbar-ui/src/components/alert_dialog.rs`, built on the **same**
`gpui_component::dialog::{Dialog, DialogHeader, DialogFooter, DialogTitle,
DialogDescription}` that `dialog.rs` already wraps.

> A §6.2 row, in the shape `native/MAPPING.md` fixes. Kept in its own file for
> the reason `popover.md` gives, and — sharper here than anywhere else in this
> tree — because the two components' *anchor ids* have to stay two namespaces
> even though their *values* are, by this item's own finding, the same.

## 0. The headline: `alert-dialog` is a variant of `dialog`, not a distinct
picture — read from the source diff, not a guess

Base UI ships `AlertDialog` as a **separate React primitive**
(`@base-ui/react/alert-dialog`, not `@base-ui/react/dialog`) for accessibility
semantics: `role="alertdialog"`, no light-dismiss on backdrop click, no
`Escape`. None of that has a second `gpui-component` implementation —
`gpui-component` is not a Base UI clone, and the one "bordered modal box"
primitive it carries is `gpui_component::dialog::Dialog`, the type `dialog.rs`
already wraps. (It *also* has its own `gpui_component::dialog::AlertDialog`,
a **third** type layered on top of `Dialog` with its own opinionated
OK/Cancel footer — read and rejected; see §1.) So the question this item was
handed was not "wrap or build" but "does `alert-dialog` need its own
primitive at all", and the answer comes from diffing the two `.tsx` files'
**compiled class lists**, not from the accessibility semantics neither side's
geometry contract can see.

`dialog.tsx`'s `DialogPopup` against `alert-dialog.tsx`'s `AlertDialogPopup`,
on the one reachable configuration of each (§3):

| | `dialog.tsx` | `alert-dialog.tsx` |
|---|---|---|
| popup | …`shadow-lg/5 `**`outline-none `**`transition-…` | …`shadow-lg/5 transition-…` (no `outline-none`) |
| header | `p-6 gap-2` | `p-6 gap-2` **`text-center sm:text-left`** (see §2) |
| footer (`variant="default"`, the only arm either reachable call site takes) | `border-t bg-muted/72 py-4` | `border-t bg-muted/72 py-4` — **byte-identical** |
| title | `font-heading font-semibold text-xl leading-none` | **byte-identical** |
| description | `text-muted-foreground text-sm` | **byte-identical** |

`outline-none` sets the CSS `outline` property — paints outside the border
box, no `ANCHORS.md` field on either side (§6 material, same as `dialog`'s
`shadow-lg/5`), geometrically inert. `text-center`/`sm:text-left` is real and
addressed in §2, and does not move a box either. Every length constant, every
radius, every colour token and every line-height `crates/crowbar-ui/src/
components/alert_dialog.rs` needs is therefore **`dialog`'s own**, restated
under this surface's own anchor ids (`alert-dialog-*`, not `dialog-*` — the
two are measured as *separate surfaces*, and `ANCHORS.md` compares snapshots
by id) rather than because a single number differs. Every constant in
`alert_dialog.rs` carries a doc comment pointing at `dialog::Dialog`'s
equivalent, and the crate's own tests assert the two crates' `pub` constants
equal each other directly (`every_length_is_the_compiled_spacing_multiple_and_
matches_dialog`).

**Answer to the brief's question, stated plainly: `alert-dialog` is a variant
of `dialog`, not a distinct surface.** It is still its own §6.2 item — its
own React file, its own `data-slot`/`data-oracle-id` namespace, its own
`crowbar-ui` module, its own `crowbar-app` surface — because `ANCHORS.md`
compares by id and a future capture of one must never silently compare
against the other's reference. But the *port* reuses `dialog`'s already-
converged values rather than re-deriving a second set from a second live
capture, because the class diff above already proves there is no second set
to derive.

## 1. What `gpui-component` offers, and why the extra `AlertDialog` type was
read and rejected

`native/vendor/gpui-component/src/dialog/alert_dialog.rs` exists —
`gpui_component::dialog::AlertDialog`, composing `Dialog` with its own
defaults: a footer built from `DialogButtonProps` (an OK button, and an
optional Cancel), centre-aligned, an optional icon. Read in full before
deciding, because it is a real §10.1 candidate. Rejected for two concrete
reasons, not a preference:

1. Its default footer shape (`.confirm()`/`.show_cancel(true)` → OK + Cancel,
   `DialogFooter::new().justify_center()`) does not match `alert-dialog.tsx`'s
   actual footer, which every live call site builds by hand
   (`review-thread-item.tsx`'s is `AlertDialogClose` render-propped to an
   *outline* `Cancel` plus a *destructive* `Delete` — no "OK" anywhere, and
   `justify-end`, not centred). Reaching for `gpui_component::AlertDialog`
   would mean overriding its opinionated footer back to something it does not
   have a name for, which is more work than the plain `Dialog` wrap below,
   for a type this port would have to fight rather than lean on.
2. It offers *more* ways to reach the same picture, not a different one —
   `AlertDialog::into_dialog` bottoms out in the same `Dialog` this crate
   already wraps, calling `.alert_dialog_role()` and building its header/
   footer through `DialogHeader`/`DialogFooter`/`DialogTitle`/
   `DialogDescription`, the identical vendor types `dialog.rs`'s own
   neutralisation targets. Wrapping the base `Dialog` directly reuses
   `dialog.rs`'s **already-tested** neutralisation technique verbatim (§3 of
   `dialog.md`) rather than adding a second layer with its own defaults to
   audit.

So `alert_dialog.rs` constructs `gpui_component::dialog::Dialog` the same way
`dialog.rs` does — `GpuiDialog::new(cx).overlay(false).close_button(false)
.p_0().bg(TRANSPARENT).border_0().border_color(TRANSPARENT).rounded(px(0.))`
— and nests its own inner div (border, radius, bg, all from `Theme`) one
level inside, exactly `dialog.rs`'s pattern.

## 2. `text-center sm:text-left` is real, and it moves nothing this contract
carries

`AlertDialogHeader`'s class list is `flex flex-col gap-2 p-6 text-center
max-sm:pb-4 sm:text-left` — `dialog.tsx`'s `DialogHeader` has no `text-align`
utility at all. `text-align` moves where a run of text sits *inside* its own
box; it does not move the box's own width or height. Every anchor under this
header is block-level with no authored width of its own (§4's
`CONTENT_SIZED` is empty, exactly as `dialog`'s is), so the header's, the
title's and the description's *boxes* are unaffected by which side the
cascade resolves to — and on every cell this port drives (at or above the
`sm` breakpoint, the convention `dialog`'s own `flex-row`/`justify-end`
footer choice and `dropdown_menu`'s `sm:` trap both already rest on),
`sm:text-left` is what wins — the same value a header with no `text-align`
class at all resolves to by default in a left-to-right document. `ANCHORS.md`
carries no `text-align` field on either side regardless (ordinary glyph
position within a box is not part of the bounds/colour/radius/border
contract §3 defines), so the class is real, named here, and not modelled.

## 3. Reachability: real code, blocked by an environmental defect this item
did not introduce

`alert-dialog.tsx` has **one** importer anywhere in `web/src`:
`features/git/components/review-thread-item.tsx`. Its
`<AlertDialogContent>` (no `className`, so the primitive's own `max-w-lg`)
nests exactly `AlertDialogHeader` → `AlertDialogTitle` + `AlertDialogDescription`,
then `AlertDialogFooter` (`variant="default"`, the only arm reached — an
outline `Cancel` and a destructive `Delete`, both `size="sm"`) — **no body
between them**, unlike `dialog`'s `add-repository-modal` reference, which has
a two-field form. `pendingDelete !== null` opens it, set by a message row's
own "Delete" menu item — reachable on **every** thread regardless of
authorship (`canDelete = isRoot ? Boolean(onDeleteThread) :
Boolean(onDeleteMessage)`, no author check, unlike `canEdit`'s).

This is real, driveable code — the opposite of `toast`'s finding (see
`toast.md`: zero producers, in any environment). A live capture was attempted
in full, not merely considered:

* The dev-fixture project's `demo` repo and its locked review workspace both
  exist and answer real requests over the daemon's own REST API — confirmed
  directly: `curl --unix-socket <sock> http://localhost/v0/projects/<p>/repos/<r>/workspaces` returned both workspaces, and `POST
  .../workspaces/<w>/threads` returned `201` with a genuine persisted
  `ThreadDTO` — not a fabricated DOM element, a real row the daemon now holds.
* The running dev webview could not reach it. Its `IndexedDB`-backed entity
  cache (`web/src/lib/persistence/entity-cache.ts`, read by
  `lib/store/workspace-list.ts`'s `buildTreeFromCache`) fails every `open()`
  with *"An attempt was made to open a database using a lower version than
  the existing version"* — logged continuously in the webview console since
  well before this item started, on every reload, unrelated to anything this
  item touched. `IndexedDB` in this app is shared **by bundle id across every
  build, including production** (a standing hazard this session did not
  create), so a newer schema written by some other session sharing the same
  webview poisons this older bundle's own open.
* The practical effect, traced rather than assumed: `useSidebarStore.repos`
  reads back empty, so `lib/store/workspace-route-guard.ts`'s
  `shouldRedirectUnknownWorkspace` treats *every* repo-scoped workspace route
  as unknown and bounces it back to the project's own `/home` —
  confirmed by navigating there directly
  (`location.hash = '#/ide/<project>/<repo>/<ws>'` inside the live webview)
  and watching the daemon access log 404 `.../home/review/*` immediately
  afterward, instead of ever reaching the repo-scoped endpoint the same
  daemon answers correctly over `curl` in the same minute.
* Resetting the shared `IndexedDB` was not attempted: the same store also
  backs the **production** app, and clearing shared storage to unblock one
  item's capture is not a call this session gets to make unilaterally.

So: **no capture, and no fabrication.** The evidence this item has instead is
of a different, load-bearing kind — the source-level class diff in §0, which
means the numbers `dialog.md`'s own live capture already converged on are
this surface's numbers too, without inventing a second live reference to
prove a fact the class diff already proves. A future item that can reach the
fixture past the `IndexedDB` defect should regenerate
`/tmp/p3-ref-alert-dialog.json` through `extractSnapshotSource` on the live
`review-thread-item.tsx` delete confirmation and confirm it reproduces
`dialog.md`'s own §0 numbers exactly, on every field the two surfaces share.

## 4. The fixture, and how it is not a guess either

`AlertDialog::fixture()` is the one reachable call site's real *source*
shape — no live pixels, but real facts, not placeholders:

* `title`/`description`: the literal strings `review-thread-item.tsx` writes
  for `deletingIsRoot && !hasReplies` — `"Delete this thread?"` /
  `"This permanently deletes the comment and its thread."`
* `body_height = 0`: **the real shape**, not a stand-in. Unlike `dialog`'s
  `add-repository-modal`, this call site nests nothing between its header and
  its footer at all.
* `footer_content_height = 28`: derived, not invented — `button::Size::Sm`'s
  own already-ported, already-tested height at the `sm` breakpoint every
  driven cell in this tree uses (`h-8 sm:h-7` → 28px), because both of this
  call site's buttons are explicit `size="sm"`, unlike `add-repository-modal`'s
  default-sized ones (`dialog`'s 32px).
* `max_width = 512`: `alert-dialog.tsx`'s own **unmodified** `max-w-lg` —
  Tailwind's stock container step, uncustomised by this app's `theme.css`
  (which redefines colour/radius/spacing, not the container scale) — because
  the one live call site passes no `className` to override it.

Popup height by the identical arithmetic `dialog::Dialog::popup_height` uses:
`2×border + header(2×24 + 20 + 8 + 20) + body(0) + footer(1 + 2×16 + 28)` =
`2 + 96 + 0 + 61` = **159px**.

## 5. What resisted — one thing `dialog.md` never had reason to find

`gpui_component::Dialog::render` sets its own outer box's `.min_h_24()`
(**96px**) *before* `refine_style` runs, and `refine_style` only overwrites a
`StyleRefinement` field this crate's own chain actually sets — `dialog.rs`'s
render never set `min_height`, because its one reachable body (172px) is
always above the 96px floor, so the floor was silently absorbed and
undetectable on every cell `dialog.md` ever drove. `alert-dialog`'s real body
is genuinely **0px**, well under the floor, and is the first cell in this
whole tree low enough to expose it: `row_layout/alert_dialog.rs`'s `empty`
test first read `96px` against a `2px` claim.

Fixed the same way every other unconditional vendor default in this tree is
neutralised — field by field, after the vendor's own default, before
`.children([popup])`:

```rust
.min_h(px(0.0))
```

**Applied to `dialog.rs` too**, for genuine correctness rather than leaving a
latent gap in already-merged code: `dialog::Dialog::render` gained the
identical line, and `row_layout/dialog.rs` gained a regression test
(`an_empty_body_is_not_clamped_to_the_vendors_min_height_floor`, driven at
`--body-height 0 --flags empty`) so the fix cannot regress unnoticed on the
surface it was *found* on, even though that surface's own reachable
reference never falls under the floor.

## 6. Reachability count and the state axis

**Live count: 1 importer, 1 reachable configuration** (module docs §3).
`AlertDialogPrimitive`/`AlertDialogClose` are re-exported but have no second
call site.

The state axis is the same shape `dialog`'s is: `loading`/`error`/`hover`/
`focus`/`selected` unmodelled (`grep -o 'hover:[a-z-]*\|focus:[a-z-]*\|active:[a-z-]*'
alert-dialog.tsx` is empty on all five named boxes); `empty` real, on the
identical arithmetic — removes the header and the footer together. No live
call site actually renders that shape (the one reachable call site always
renders both), but the primitive itself permits it exactly as `DialogPopup`'s
does, and it is declared for the reason `dialog`'s own `empty` arm and
`sheet`'s whole module both give: "port it and say so".

## 7. Verdict

**Strict parity is reached on every field the contract carries, by
construction**: the port's values are `dialog`'s own already-converged
values (§0's class diff), restated under this surface's own anchor
namespace, plus one real, derived difference (`body_height = 0`,
`footer_content_height = 28`) that follows from this call site's own actual
shape rather than from a divergent class. No property resisted styling.
What resisted, once, was the vendor's own `min_h_24()` floor — caught by
this item's own `empty` test, fixed in both `alert_dialog.rs` and (for
correctness) `dialog.rs`, and guarded by a regression test on each.

No live pixel reference was captured (§3) — this file is therefore evidence
by class-diff and by shared code, not by a second `/tmp/p3-ref-*.json`. There
is no `/tmp/p3-ref-alert-dialog.json` from this pass; fabricating one would
be exactly the "geometry matching an expectation is not evidence" mistake
this item's own brief exists to refuse.
