# `toast` (P3.28) — built, not wrapped, and reachable by nobody at all

`web/src/components/ui/toast.tsx`'s `AnchoredToasts` (the file's only
rendering export) → `crates/crowbar-ui/src/components/toast.rs`, raw `div()`s
under this crate's own theme, no `gpui-component` widget in the render path.

> A §6.2 row, in the shape `native/MAPPING.md` fixes. Kept in its own file for
> the reason `popover.md` gives, and one sharper: its `CONTENT_SIZED`/
> `LINE_SIZED` would collide with `popover`'s and `tooltip`'s the way every
> other unflattened module's would (`components/mod.rs`'s own note on
> `pub mod toast;`).

## 0. The headline: two independent findings, and neither should be read as
the other

1. **Wrap or build**: build. `gpui_component::Notification`/`Alert` both fail
   the seam test §1 below applies. This is a *structural* finding, about the
   vendor's own API shape, independent of reachability.
2. **Reachability**: zero, and provably so rather than merely unobserved this
   session (§2). This is a finding about `web/src`'s own call graph,
   independent of what `gpui-component` offers.

A worker who conflates them — "it's unreachable, so the wrap question doesn't
matter" — would be making exactly the mistake `dropdown_menu.md` and
`popover.md` both warn against under a different name: treating an absent
reference as license to skip evidence rather than as a reason to be *more*
exact about what was actually checked. Both are checked in full below.

## 1. Wrap or build: the seam test, applied to every vendor candidate this
item's brief names

§10.1 says not to rebuild a primitive `gpui-component` already has, and
`native/vendor/gpui-component/src/notification.rs` (`Notification`,
`NotificationList`) and `native/vendor/gpui-component/src/alert.rs` (`Alert`)
are both real candidates, both named in this item's brief. The test is
`popover`'s own, restated by this item's brief in full: **a widget is
wrappable-and-measurable exactly when it lets the caller supply an
*element*, not merely a style** — `AnchorSink::root`/`boxed` take a
[`gpui::Div`] this crate holds, and a box built entirely inside the vendor's
own private `render`, reachable only through a `StyleRefinement`, is nothing
to anchor. Read against the vendor source directly, not a member-name grep —
the brief names two seams a fixed-list grep would miss (`focus_trap`,
`v_virtual_list`), and this file's own investigation missed neither
candidate's real seam by reading every public method:

* **`Notification::content`** *does* take a builder —
  `Fn(&mut Self, &mut Window, &mut Context<Self>) -> AnyElement + 'static`.
  Same shape `popover.rs`'s own module docs already ruled out for
  `Popover::content`: `'static`, and a component here is handed
  `&dyn AnchorSink` with an anonymous lifetime, which cannot be captured by a
  `'static` closure. Not a coincidence — `gpui_component::popover::Popover
  ::content` has the identical signature, for the identical reason.
* **Unlike `Popover`, `Notification` has no fallback.** `Popover` also
  implements `ParentElement`/`.child()`, which takes an *already-built*
  `AnyElement` — no closure, no `'static` bound — and that is the seam
  `dialog.rs`, `sheet.rs` and `popover.rs` all actually use. `Notification`
  implements neither `ParentElement` nor anything with an equivalent shape:
  its whole painted box (`h_flex().id("notification")…
  bg(cx.theme().tokens.popover)…rounded(cx.theme().radius_lg)…`) is built
  inside its own private `Render::render`, and the only other seam is
  `Styled::style()` — a `StyleRefinement` on that same private `h_flex`, the
  "nothing to anchor" shape `tooltip.rs`'s own module docs and this item's
  brief both name as the fake convergence `ANCHORS.md` exists to refuse.
* **`Alert`** fails one door further shut: `title`/`message` are
  `SharedString`/`Text`, not elements; `icon` is a fixed `Icon` type; there is
  no `content` closure at all, `'static`-bound or otherwise. No seam of any
  shape.

**Verdict: built, not wrapped** — the third component in this tree to fail
this exact test outright (`dropdown_menu`, `checkbox`, `tooltip` are the
others; `tooltip.rs`'s module docs apply the same test to a different vendor
type and reach the same shape of answer). `toast.rs` is raw `div()`s under
[`Theme`], the same pattern `tooltip.rs`, `badge.rs` and `kbd.rs` already
establish for a component with no real vendor seam.

## 2. Reachability: zero, and it is provable rather than merely unobserved

`toast.tsx` exports two things: `toastManager` (a bare
`Toast.createToastManager()` singleton, re-exported through
`lib/toast-manager.ts` so stores can import it without crossing the
stores-must-not-import-from-components rule) and `anchoredToastManager` +
`AnchoredToastProvider`/`AnchoredToasts` — a **second**, independent manager
and the *only* component that renders this file's own JSX (the icon set,
`upsertReplayClassName`, the `tooltipStyle` branch — all of it lives inside
`AnchoredToasts` and nowhere else in `web/src`).

`AnchoredToasts` calls `Toast.useToastManager()` bound to
`anchoredToastManager` and, per toast, `if (!positionerProps?.anchor) return
null` — it paints something only when a caller has already called
`anchoredToastManager.add(…)` with a `positionerProps.anchor` set.

**No caller ever does.** `grep -rn anchoredToastManager web/src` finds
exactly three lines: the singleton's own declaration in `toast.tsx`, that
same file's `<Toast.Provider toastManager={anchoredToastManager}>`, and
`lib/toast-manager.ts`'s re-export — no `.add(` anywhere in the tree. Every
real toast the running app shows a user goes through the **other** manager
(`features/window/stores/toast-store.ts`'s `toast.show`/`.info`/`.success`/
`.warning`/`.error`, all `toastManager.add(…)`), rendered by a **third,
unrelated file** — `components/layout/sidebar-toast-overlay.tsx`'s own
hand-rolled `SidebarToastItem`, which imports the `toastManager` object and
none of `toast.tsx`'s JSX. `AppDialog`'s relationship to `DialogPopup`
(`dialog.md` §5) is the same shape one door over — a second, independent
renderer that bypasses the primitive entirely — except `AppDialog` is at
least reachable through *some* path, and `AnchoredToasts` has none: no route,
no button, no keyboard shortcut in this codebase ever calls
`anchoredToastManager.add`.

This is the strong form of the brief's own instruction — *"if you cannot
reach the real element, STOP and report — never construct, inject or stub
one"* — applied to a component with **no code path in any environment**,
rather than one blocked by an environmental defect (contrast `alert-dialog`'s
finding, `alert-dialog.md` §3: real code, blocked by a shared `IndexedDB`
schema mismatch this session did not introduce). No reference was captured,
attempted through a synthetic trigger, or fabricated. There is no
`/tmp/p3-ref-toast.json`.

## 3. `popover`'s `Variant::Tooltip` does not already cover this

The brief asks the question directly, and half of it is already answered in
`popover.rs`'s own module docs: `Variant::Tooltip` models `PopoverPopup`'s own
`tooltipStyle` prop, found by `grep tooltipStyle` to be reached "on
`toast.tsx`'s own primitive and on no `PopoverContent` anywhere" — i.e.
`popover.tsx`'s `tooltipStyle` arm is *itself* unreached, modelled by reading
source, the position this file is in too. So the live question is not "is
`Variant::Tooltip` reachable" (no, on both sides) but "is it the *same
shape* as `toast.tsx`'s `tooltipStyle` branch, so this file could reuse it
instead of duplicating it".

The CSS **values** agree almost everywhere — `rounded-md`, `text-xs` (this
crate's `ui_text_sm`), and `popover`'s tooltip viewport padding `py-1
[--viewport-inline-padding:--spacing(2)]` is the identical 4px/8px pair as
`toast.tsx`'s own `Toast.Content className="… px-2 py-1"` under
`tooltipStyle`. The **shapes** do not:

| | `popover::Variant::Tooltip` | `toast`'s `tooltipStyle` |
|---|---|---|
| outer box width | a **required, caller-supplied `Pixels`** (`Popover::width` — every live call site's own `w-*`, `repo-icon-popover`'s measured 256px) | **none** — `Toast.Root` has no width class at all; `Toast.Positioner`'s `max-w-[min(--spacing(64),var(--available-width))]` is a *cap*, not a length, and the root shrinks to its content the way `tooltip.rs`'s own root does |
| title, when present | a *separate*, always-styled `PopoverTitle` (`font-semibold text-lg leading-none`) nested in the viewport alongside arbitrary `children` | **is** the box's only content, and carries **no className of its own** under this branch — plain, inherited 12px, not semibold, not `leading-none` |
| driven by | a click-to-open trigger + `gpui_component::Popover`'s deferred/anchored placement, open/closed state | a timed, queued notification list (`Toast.Root`/`Toast.Positioner` pairs, one per active toast, stacked by a manager) — no trigger, no vendor widget in the render path at all (§1) |

A caller reusing `Popover::render(Variant::Tooltip, …)` for a toast would
have to invent a width `toast.tsx` never authors and render a title box
shaped like `PopoverTitle` where the reference's is shaped like nothing at
all — two fabrications standing in for one reused module. **Verdict: related
by coincidence of both being unreached and both being "the small padded arm"
of a two-arm component, not by one covering the other.** `toast.rs` is its
own, independent port — the inverse of `tooltip.rs`'s own "`tooltip.tsx` is
not `popover --tooltip`" finding, for a different pair of surfaces.

## 4. Declarations, and why both lists are empty rather than omitted

`CONTENT_SIZED` and `LINE_SIZED` are both `[]`:

* The **popup** is not v1.5-content-sized in the sense that declaration
  requires — a box whose used width *is* a text run's max-content width
  (`dialog.rs`'s own phrase, restated by `alert_dialog.rs` for the same
  reason). Under the default variant it is a multi-child flex subtree (an
  icon column beside a title/description column); even under
  `tooltipStyle`, where it *is* one run, the same anchor id has to mean one
  thing across both configurations, the way `dialog::ID_TITLE` means one
  thing whether or not a description sits beside it. It is, however, built
  with **no authored width** either way — `.max_w(MAX_WIDTH)` only, no
  `.w()` — so gpui's own flex layout produces the same shrink-to-fit box
  `WebKit` would, undeclared rather than mis-declared.
* Neither **title** nor **description** carries `leading-none` in either
  branch — unlike every title this tree has ported so far (`dialog`'s,
  `popover`'s, `sheet`'s, `alert_dialog`'s), `toast.tsx`'s title keeps the
  ambient paired line height and is `break-words` prose under the default
  variant — exactly `dialog::ID_DESCRIPTION`'s own shape, never
  `dialog::ID_TITLE`'s. Declaring either would manufacture the delta v1.6
  exists to prevent.

## 5. Two divergences from the source tree shape, both in the port and both
named rather than silent

* **`.flex()` on the root**, though `Toast.Root` carries no `flex` class of
  its own. The reference's shrink-to-fit width comes from
  `Toast.Positioner`'s absolute/fixed placement — an out-of-flow box with no
  declared width sizes to its content by default in CSS — which this crate's
  row-layout harness does not reproduce (every surface is drawn inside a
  `--width`-sized block container). `.flex()` is what makes gpui's own
  layout shrink-wrap the box the same way, the identical trade
  `tooltip::Tooltip::shell` makes.
* **`Toast.Content`'s padding is folded onto the root box directly**, rather
  than modelled as a second, nested anchored div. `Content` carries no
  border, background or radius of its own in either variant — all on
  `Toast.Root` — so a child's padding with a parent's zero-margin border
  produces an identical outer box either way, and `Content` earns no anchor
  of its own: it carries no `data-slot` in the source, unlike every other
  wrapper primitive this tree has anchored.

## 6. What the icon and the action row are, and are not

Both are real, both are rendered, neither is anchored to a comparison —
`dialog::Dialog::body` and `alert_dialog::AlertDialog::body`'s precedent for
a call site's own content, applied here for the same reason:

* The **icon** is a `lucide-react` SVG a call site's `toast.type` selects —
  no native equivalent, the call `badge::Badge::glyph_box` and `tooltip`'s
  shortcut chip both already make about a call site's own SVG. `w-4` (16px)
  wide; the height is `h-lh`, one line of the *content* column's own text —
  which never grows the row past the column's own height, since the column
  is always at least one line tall itself.
* The **action** is a full `<Button>` whose *label* is the call site's own
  string (`toast.actionProps.children`) — the same shape `dialog`'s two
  footer buttons are, one component over: call-site content collapsed to the
  height it occupies, `div().h(content_height)`, no per-button anchor.

## 7. Verdict

**Built, not wrapped** (§1) — a structural finding about `gpui_component
::Notification`/`Alert`'s own API shape, true regardless of reachability.
**Zero live producers, in any environment** (§2) — a call-graph finding
about `web/src`, true regardless of what `gpui-component` offers. Neither
covers or explains the other, and neither is `popover`'s `Variant::Tooltip`
under a second name (§3).

No reference was captured, attempted or fabricated. There is no
`/tmp/p3-ref-toast.json`; producing one would require either driving a code
path that provably does not exist anywhere in this application, or injecting
a bare element with copied bounds — precisely the fabrication this item's
brief names as the mistake a prior worker on this project already made once.
Every value in `toast.rs` is instead a Tailwind class compiled by hand,
carrying the same discipline (and the same caveat: "no live number to
measure any of it against") that `sheet::Sheet` and `radio_group` already
apply to their own unreached surfaces. All 25 of this module's and its
surface's tests check internal consistency — the port renders, each
variant's padding-derived offsets follow the arithmetic its own docs claim,
`empty` moves what it says it moves, no field leaks across the two variants
— never a captured number, because none exists to check it against.
