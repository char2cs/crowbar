# `radio-group` (P3.22) — built, not wrapped; the item's zero-reachability finding

`web/src/components/ui/radio-group.tsx` (`@base-ui/react/radio` +
`@base-ui/react/radio-group`) →
`crates/crowbar-ui/src/components/radio_group.rs`, built from raw `div()`s.

> A §6.2 row, in the shape `native/MAPPING.md` fixes. Kept in its own file
> because P3 runs several workers in parallel and one appended table is one
> conflict per item.

**There is no `/tmp/p3-ref-radio-group.json`.** `radio-group.tsx`'s only
importer, `web/src/features/git/components/merge-popover.tsx`, is
unreachable in this item's dev environment — see §1. No reference is
fabricated in its place; the component is ported and the zero is reported,
as the item's brief asks.

**Live count: 0 of 1** — one importer, unreachable.

## 0. The seam test, applied

`native/vendor/gpui-component/src/radio.rs` has a `Radio` and a `RadioGroup`.
`Radio` **does** implement `ParentElement`, which is the shape this item's
brief specifically named as easy to miss on a name-only grep — so, unlike
`tooltip`, it earned a closer reading before the verdict.

What `ParentElement` reaches is not the circle:

| seam | what it reaches | wrappable? |
|---|---|---|
| `Radio::new(id)` | allocates its own `base: Div` **internally** — this crate never constructs or holds it | — |
| the circle itself (`div().relative()…size_4()…rounded_full().border_1().border_color(…).bg(…)`) | built **inside** `RenderOnce::render`, unconditionally, no `appearance(false)`-style flag | no |
| `.child()` / `.children()` (`ParentElement`) | appends to a `Vec<AnyElement>` that lands in a **second, separate** box — the label area, rendered only `when(!children.is_empty() \|\| label.is_some())` | reaches a box, but not the circle |
| `Styled::style()` | refines `self.base` — the `h_flex` row wrapping circle **and** label — not the circle | no |

The one box the reference needs pixel-identical — `border-input`,
`bg-background`/`bg-input/32`, `rounded-full` — is never a `Div` this crate
holds, for the identical structural reason `tooltip.rs` documents: **the seam
is real, but it opens onto the wrong box.**

A second, independent tell, found by comparing the two DOM shapes rather than
only the Rust API: `gpui_component::Radio` **bundles a label** — the
React primitive does not. `radio-group.tsx`'s `Radio` is
`RadioPrimitive.Root` alone; every live label
(`merge-popover.tsx`'s `<label><Radio /><div>…</div></label>`) is the call
site's own sibling markup, never a child the primitive lays out. Wrapping the
vendor's `Radio` would therefore reproduce a shape `radio-group.tsx` does not
have, on top of not being able to anchor the shape it does.

**Verdict: built**, the same way `checkbox` and `dropdown_menu` are.

## 1. Reachability — measured, and it is zero

`radio-group.tsx`'s only importer is `merge-popover.tsx`, and that popover
needs a workspace that is a **child branch with an unprotected local
parent**. This is a stronger requirement than `popover`'s own
`commit-popover`, which needed only a dirty worktree — this item's dev
environment has that (an `oracle-fixture` project, one workspace, dirty:
`staged-file.ts`, `untracked-new-file.ts`, a modified `a.ts`) — but that
workspace **is** the repo root (`home`), not a branch, and Crowbar's own
workspace model makes `home` a place with no parent to merge into by
definition.

Checked rather than assumed:

* `Switch workspace` (the quick picker) lists exactly one entry:
  `oracle-fixture / home`.
* `Switch project` → `Projects` lists exactly one project, no others
  imported.
* The `Workspaces` panel's own tree, under the one project row, is empty —
  no child branch, no "new branch" affordance found in it.
* `document.body.textContent` contains the substring `merge` **zero** times
  anywhere in the running app, confirming no merge-adjacent UI is mounted at
  all in this state (not merely scrolled off the sidebar carousel).

No git or filesystem workaround was taken either: the `.crowbar` state
backing this dev instance's `oracle-fixture-demo` project does not correspond
to a lightweight fixture repo the way its name suggests, and manufacturing a
branch on disk would not register as a workspace without the daemon's own
provisioning path — which is exactly the "no legacy migration, no
side-channel state" territory this item's constraints keep off limits
(`native/oracle/**`, the daemon's own git operations are not this item's to
drive).

**Live count: 0 of 1.** Ported anyway, per the brief.

## 2. How the values were measured, given zero reachability

Not inferred from the class name. `radio-group.tsx`'s own compiled class
strings were injected as a throwaway element into the *live* app's DOM —
which already carries the real, compiled Tailwind sheet those classes
resolve against — and read with `getComputedStyle`, then removed
immediately. No component was mounted through React; no reference JSON was
written from this, and none is claimed as one. It is the same "read the
compiled rule, not the class name" discipline `native/MAPPING.md` prescribes
for a live reference, applied without a live mount to take one from.

Confirmed independently rather than assumed from the shared substring:
`radio-group.tsx`'s circle carries `checkbox.tsx`'s **exact**
`dark:not-data-checked:bg-input/32` rule, and the injected measurement
reproduces `checkbox.rs`'s own documented on/off background pair to the
token:

| | light | dark |
|---|---|---|
| off | `oklch(0.994 0.001 106.4)` | `oklab(1 0 0 / 0.0256)` (`input/32`) |
| on | `oklch(0.994 0.001 106.4)` | `oklch(0.239 0.002 106.5)` (`background`) |

The light column is the same colour twice — `selected` cannot fail there,
`checkbox.rs`'s exact finding, reproduced independently on this control.

## 3. Values — the circle

| React / Tailwind | Compiles to (injected measurement) | gpui / `crowbar-ui` | Oracle |
|---|---|---|---|
| `size-4.5 sm:size-4` | `18px` / `16px` | `radio_group::extent(Breakpoint)` | `bounds` |
| `rounded-full` | `border-top-left-radius: 340282346638528859811704183484516925440px` — **`f32::MAX`**, not gpui's `rounded_full()` preset of `px(9999.)` | `px(f32::MAX)` | `radius` |
| `border border-input` | `1px`, `oklch(1 0 0 / 0.08)`, unconditional (present in both `data-checked` and `data-unchecked`) | `.border(BORDER_WIDTH).border_color(theme.input)` (unless `--invalid`, see §4) | `border.w` = 1 exactly |
| `bg-background`, `dark:not-data-checked:bg-input/32` | see §2's table | `Radio::background` | `bg` |
| `shadow-xs/5`, removed by `[[data-disabled],[data-checked],[aria-invalid]]:shadow-none` | present when unchecked/enabled/valid | not painted | **§6: no field** |
| `not-data-disabled:not-data-checked:not-aria-invalid:before:shadow-[…]` | a conditional `::before` inset shadow | not painted | **§6: no field**, and a pseudo carries no anchor either way |

**`rounded-full` is `f32::MAX`, confirmed a third time on a form control
rather than a decorative shape.** `avatar.rs` and `switch.rs` establish the
trap; this is independent confirmation on `radio-group.tsx`'s own compiled
rule, not an assumption carried over from either.

## 4. `aria-invalid` — real, unreached, and named in advance by `checkbox.rs`

`radio-group.tsx` carries `aria-invalid:border-destructive/36`,
`focus-visible:aria-invalid:border-destructive/64`,
`focus-visible:aria-invalid:ring-destructive/48` and
`dark:aria-invalid:ring-destructive/24` — **byte-identical substrings** of
`checkbox.tsx`'s own rules. `checkbox.rs`'s module docs name this exactly:
*"`select`, `checkbox`, `radio-group` and `textarea` carry the same four
rules and will hit this again."* They do, and it did: `border_color` on
`crowbar_ui::components::radio_group::Radio` reproduces `checkbox.rs`'s
`border_color` chain field for field, driven by `--invalid` on the surface
rather than by §8.3's `error` flag, for the identical reason (every surface
must declare `error` unmodelled, `surface.rs`'s own workspace invariant).
**No reference either way**: `radio-group.tsx` has zero live call sites,
let alone one passing it.

## 5. The indicator — unanchored, `checkbox.rs`'s precedent, not re-derived

`data-unchecked:hidden` is `display: none`. `native/oracle/ANCHORS.md` v1.11
already settles what that means for a snapshot: the DOM extractor keeps a
`display: none` element mounted and emits a zero-rect record; GPUI's simply
never exists, because prepaint never arrives for it. `radio-group.tsx`'s
`Indicator` carries **no `data-oracle-id`**, `checkbox.tsx`'s exact fix for
its own indicator, applied here rather than re-derived — see that file's own
comment, unchanged in spirit. The fill (`data-checked:bg-primary`) and the
inner dot (`before:bg-primary-foreground`, a `::before` that is not
`inset: 0`, so not eligible for `ANCHORS.md`'s pseudo-backed shortcut
either) are painted, by both sides, and measured by neither.

## 6. A group renders three; this surface anchors one

Every live `RadioGroup` (`merge-popover.tsx`'s three merge strategies) holds
three `<Radio>`, each carrying the identical static `data-oracle-id="radio"`
— `radio-group.tsx` has no per-index id, on purpose: assigning one is the
**call site's** job, `dropdown_menu`'s own precedent for `ID_ITEM` ("a menu
with several items names them at the call site... `dropdown-menu.tsx` cannot
invent an index it does not have"). With zero reachability there is no
reference to check a naming scheme against, so this surface takes the
simpler, already-precedented shape rather than inventing one: the group
anchors its own root (`radio-group`) and **one** representative `Radio`
(`radio`). Further options in a live group are real and would paint,
unanchored, the same way `popover`'s call-site body does — this surface does
not render them, since nothing would exercise the choice.

## 7. What resisted, precisely

Nothing resisted *styling* — every field in §3 and §4 has a token or a
measured constant behind it, confirmed by injected measurement rather than
assumed. What resisted was **capture**: zero reachability means none of it
has been checked against a live snapshot, and none of it can be, in this
item's environment, without touching state this item's constraints keep off
limits. This item does not run the oracle or the differ in any case — the
convergence verdict, when a reference eventually exists, belongs to whoever
takes it.
