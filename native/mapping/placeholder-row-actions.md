# `placeholder-row-actions` (P3.62)

`web/src/components/layout/placeholder-row-actions.tsx` →
`crates/crowbar-ui/src/components/placeholder_row_actions.rs`,
`crates/crowbar-app/src/surfaces/placeholder_row_actions.rs`,
`crates/crowbar-app/src/row_layout/placeholder_row_actions.rs`.

> Cluster 7, "small tree-row controls" (`native/mapping/layout-denominator.md`
> §8): `workspace-inline-input.tsx` · `placeholder-row-actions.tsx`.

**No captured reference.** Same posture as `workspace-inline-input.md` §0's:
the `data-oracle-id`s are added to the React source as part of this item, and
the values below come from the app's own compiled Tailwind and a probe
element in the live document (`native/MAPPING.md`'s method). **Verdicts are
the queue's** — no snapshot JSON was captured or fabricated.

## 0. The two real reason strings, and what gates the second button

`placeholderReason(workspace)` (`web/src/lib/workspace/placeholder.ts`) has
**three** arms, but only two are reachable — the third needs a persisted
`lastError` the app never writes (spec §4/B7, and `inline-error.md`'s own
"no reference" finding is the identical shape for a different unreachable
branch):

| Arm | Reachable? | Detach… rendered? |
|---|---|---|
| `heldByPath` set — `` `{branch}` is checked out at {path} — detach it… `` | **yes** | yes |
| `lastError` set — `` Crowbar couldn't set up `{branch}`: {lastError} `` | **no persisted field** | no |
| neither — `` Crowbar couldn't set up `{branch}`. Retry to provision it. `` | **yes** | no |

The surface's `--held` option selects between the two reachable arms and
gates the Detach… button and its anchor in lockstep with `workspace.heldByPath`
— the same condition the real component reads for both.

## 1. Two composed `<Button>`s, `inline-error`'s precedent twice

`inline_error::InlineError`'s own module docs establish why a call site
cannot nest a second `Button::render` inside another surface: `AnchorSink::
root` clears the driver's anchor registry as it enters `prepaint`, so a
nested root would discard every anchor laid out before it. This component
has the identical shape and the identical fix, generalised to **two**
variants of the composed control in one surface for the first time in this
port: Retry (`variant="outline" size="sm"`) and Detach… (`size="sm"`,
`variant` unset — `cva`'s own default) are both built from `button`'s own
public values (`Size::Sm`'s extent/padding/gap/radius, `Variant::Outline`'s
and `Variant::Default`'s colours), reused rather than re-derived
(`both_buttons_reuse_the_button_primitives_values`).

**`button.rs`'s own module docs say no live call site renders a `Button`
with a label** ("142 `<Button` elements in `web/src/`, none of them with
[a label] — `Label` is closed… so labelled controls are hand-built"). That
claim is wrong, and not only by this item's own two: a grep for `<Button`
across every non-test `.tsx` file turns up dozens of literal text children —
`unsaved-changes-dialog.tsx`'s "Cancel"/"Discard"/"Save",
`error-boundary.tsx`'s "Try again", `oobe-screen.tsx`'s "Continue", and
(closer to home) two **already-ported** components in this exact directory:
`repo-import-dialog.tsx`'s "Import" button
(`repo_import_dialog.rs`'s own module docs: *"an `Import` button —
substantial, but every bit of it is [call-site content]"*) and `detach-
holder-modal.tsx`'s "Cancel"/"Detach" pair, both landed before this item and
both real, non-test call sites `button.rs`'s survey should already have
counted. **Both of those resolved it the other way**: `dialog::Dialog::
footer`'s own precedent — collapse the footer to one opaque, unanchored
content-height box, never anchor the individual buttons — which is what
`repo_import_dialog.rs` and `detach_holder_modal.rs` both do. This item is
the first to take `inline-error`'s alternative path instead (compose from
`button`'s own values, anchor each one individually), which is why it is
argued in full here rather than pointed at as settled: two real,
already-merged counterexamples already existed and neither this file's
`inline-error` precedent nor `button.rs`'s "no live call site" survey named
them. Not a defect in this item's own scope (`button.tsx` is untouched
here), but worth recording precisely rather than reproducing the stale claim
at whatever scale happened to be convenient: a future worker auditing
`button`'s own `CONTENT_SIZED`/v1.5 posture, or choosing between the two
resolutions above for a new call site, should read this note first.

## 2. The reason line wraps, measured at the real 262px detail width

`workspace-tree-item.tsx`'s own detail wrapper is `mx-1.5 mb-0.5
rounded-b-lg px-2.5 pb-2 pt-0.5` around the 294px sidebar: `294 − 2×6(mx) −
2×10(px) = 262`. Probed at that width with the real strings (a representative
branch/path filled in):

| Reason | Lines | `h` |
|---|---|---|
| held (`` `fix-auth-bug` is checked out at /Users/dev/… `` — one long sentence) | 4 | 76 |
| generic (`` Crowbar couldn't set up `fix-auth-bug`. Retry… ``) | 2 | 38 |

So — unlike `inline-error.tsx`'s single unbreakable detail run —
`ID_REASON` is a plain wrapped paragraph at every reachable length,
`dialog::Dialog::description_box`'s own shape. It takes neither v1.5 nor
v1.6.

## 3. Values

| React / Tailwind | Compiles to | gpui / `crowbar-ui` | Oracle |
|---|---|---|---|
| `gap-1.5` (panel; reused for the action row's own gap) | 6px | `GAP` | compared |
| `text-xs` (reason) | this crate's `--ui-text-*` trade (`native/MAPPING.md`): `text-xs` is `theme.ui_text_sm`'s number (12px), **not** `theme.ui_text_xs`'s (11px) | `REASON_TEXT_SIZE`, pinned against `theme.ui_text_sm.value()` directly rather than the name that sounds right | compared |
| `leading-relaxed` (reason) | 1.625, unredefined by `theme.css` | `REASON_LINE_HEIGHT` | compared |
| `size="sm"` (both buttons) | `h-8 sm:h-7`, no authored width | `Size::Sm`, read not copied | compared |
| `variant="outline"` (Retry) | `border-input bg-popover text-foreground` | `Variant::Outline`, read not copied | compared |
| `variant` unset (Detach…, `cva`'s default) | `border-primary bg-primary text-primary-foreground` | `Variant::Default`, read not copied | compared |

## 4. Declarations

`CONTENT_SIZED = [ID_RETRY, ID_DETACH]` — `size="sm"` authors no width, so
each button's used width is its own label's max-content width, the
`inline-error` retry-control finding, twice. `LINE_SIZED = []` — `size="sm"`
authors `h-8 sm:h-7`, `badge`'s rule (also `inline-error`'s retry control).
The reason line carries neither (§2).

## 5. The state axis

**All six §8.3 flags are unmodelled.** `PlaceholderRowActions({ workspace })`
takes no `className` and spreads no props on its root, the reason `<p>` or
the action row, and none of those three carries a `hover:`/`focus:`/
`data-active` rule of its own — the two rules the composed buttons carry
belong to `button`'s own surface, `inline-error.rs`'s identical exclusion for
its own composed retry control. `empty` has no seam either: nothing on this
surface is ever absent for want of content — the reason string always has
one of its two reachable shapes, and the Detach… button's presence is
`--held`, not `empty`.

So `Params::no_state_axis()` returns `true`, held by `surface.rs`'s own
`no_surface_declares_its_entire_state_axis_unmodelled` — `workspace_branch_
icon.rs`'s own declaration is the precedent this one follows, and the
biconditional invariant this item's own first `cargo test --workspace` run
caught the omission of before it was added.

## 6. What is not ported

| Thing | Status |
|---|---|
| both buttons' `hover`/`focus`/`active` | **absent** — `button`'s own surface's business, composed from its *resting* values only |
| `onPointerDown={(e) => e.stopPropagation()}` on both buttons | **absent** — interaction, not a static cell's |
| the third (`lastError`) reason arm | **absent, no persisted field** — see §0 |

## 7. Reachability

One importer: `workspace-tree-item.tsx`, rendered inside the placeholder
row's expanded detail section (`showPlaceholderDetails`).
