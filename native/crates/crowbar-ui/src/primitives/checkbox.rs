//! `checkbox` — a real `selected` state on **one** recorded field, in **one** of
//! the two themes.
//!
//! The native half of `web/src/components/ui/checkbox.tsx`, which is four
//! elements: a `<button role="checkbox" data-slot="checkbox">` box, its
//! `::before` overlay, a `<span data-slot="checkbox-indicator">` fill, and the
//! `<svg>` tick inside that. Every value below came out of the app's own
//! `index.css` through its own Tailwind 4.3.0 — see `native/mapping/checkbox.md`.
//!
//! # Exactly one recorded field separates on from off, and only in dark
//!
//! Measured on the live pair (`/tmp/p3-ref-checkbox.json` against
//! `/tmp/p3-ref-checkbox-selected.json`), captured from the commit popover's file
//! list:
//!
//! | field | off | on |
//! |---|---|---|
//! | `checkbox.bg` | `#ffffff07` (`dark:bg-input/32`) | `#1f1f1eff` (`bg-background`) |
//!
//! Nothing else moves. `bounds`, `radius`, `border.w`, `border.color` and
//! `visible` are identical on both cells.
//!
//! **And in the light table nothing moves at all.** The rule that carries the
//! difference is `dark:not-data-checked:bg-input/32` — a `dark:` variant. Below
//! it the box is `bg-background` in *both* states, so **`--flags selected` on a
//! light cell of this surface cannot fail**: every field the contract records is
//! identical, and a 0-delta run there proves nothing. Said plainly here and in
//! the surface's caption rather than left for a reader to infer from a passing
//! run. The live app is dark, which is the only reason the captured pair differs
//! at all.
//!
//! # The indicator is deliberately **unanchored**, and this is the interesting
//! decision on the component
//!
//! The fill that turns the box green when checked is a real element with a real
//! box — measured at `(0, 0, 16, 16)` relative to the root, `bg-primary`, radius
//! 4, no border. Anchoring it would put the `bg-primary` fill and the tick's
//! geometry under the differ, which is a lot of coverage to leave on the table.
//! It is still wrong, and the reason is structural rather than a matter of taste:
//!
//! **`data-unchecked:hidden` is `display: none`, and the two extractors disagree
//! about what a `display: none` element is.**
//!
//! | | an anchor at `display: none` |
//! |---|---|
//! | `web/src/lib/oracle/extract.ts` | **emitted.** base-ui keeps the indicator mounted, so the walk finds it; `oracleIsVisible` returns `false` and `getBoundingClientRect()` returns **all zeros**, which §4's root-relative arithmetic turns into `bounds: { x: −330, y: −406, w: 0, h: 0 }` — the *viewport* origin expressed against the root |
//! | `crowbar-driver` | **absent.** `ANCHORS.md` §6 says so in as many words: "`display: none` is caught implicitly — prepaint never arrives and the anchor is simply absent" |
//!
//! So on the resting cell the reference would carry two anchors and the port one,
//! and the differ would report an anchor present on one side only — a delta whose
//! cause is the contract rather than the port, on the cell that is the *default*
//! for this surface. Verified on the live element rather than reasoned about:
//! unchecked, `childNodes` still holds the indicator,
//! `getComputedStyle(indicator).display` is `"none"`, and its
//! `getBoundingClientRect()` is `{x: 0, y: 0, width: 0, height: 0}`.
//!
//! The two repairs available are both worse than the omission. Rendering a
//! zero-area box at the root's negated viewport position on the native side would
//! be **writing the reference's output into the port** — the knob P3.2 refused for
//! `tab-indicator` and P3.1 refused for `--class-radius`, in its purest form,
//! since the number is literally a coordinate only the reference can know.
//! Rendering the indicator unconditionally and letting `visible` carry the
//! difference is worse still: the boxes then disagree on `x`, `y`, `w` **and**
//! `h` as well, four manufactured deltas instead of one.
//!
//! `ANCHORS.md` v1.8 offers no way out either. A surface may declare its anchor
//! set "only when the set is a property of the surface rather than of the cell",
//! and this set is a function of the cell — exactly `git-status-row`'s
//! `git-row-dir`. So the honest shape is the one taken: the root carries the only
//! anchor, the fill is painted and not measured, and the omission is written down
//! here, in `native/mapping/checkbox.md` and in the surface's caption instead of
//! being discovered as a mystery delta.
//!
//! **This is a real hole in the contract, not just in this port**, and it is the
//! second of its kind after v1.7's opacity split: `ANCHORS.md` §6 states the GPUI
//! side's behaviour for `display: none` and says nothing about the DOM side's, and
//! the two do different things. It is reported rather than patched — this item may
//! not edit `native/oracle/`.
//!
//! # The tick is a box with no path, which is house style
//!
//! P3.8 established it on `crowbar-mark` and `sidebar-toggle-icon`: an `<svg>`'s
//! `stroke`, `stroke-width` and path data have **no field in the contract**, and
//! an `<svg>` has element children rather than text nodes so it emits no `fg`
//! either. The tick is therefore rendered as its box — `size-3.5 sm:size-3`,
//! measured live at `12 × 12` at `(2, 2)` relative to the root — carrying the
//! colour it would stroke with, and no path. Both indeterminate and checked
//! glyphs are the same box, which is why the two are one enum arm apart and not
//! two geometries.
//!
//! # `border` is **1px on every cell** — `button`'s side of the trap
//!
//! `checkbox.tsx` writes a bare `border`, so `border.w` is `1` in every state and
//! `ANCHORS.md` v1.1 compares it **exactly**. The variants change only the colour.
//! The mirror trap is on the same component: `focus-visible:ring-2` is a
//! **box-shadow**, so a focused box's `border.w` is still 1, never 3.
//!
//! # `rounded-[.25rem]` is 4px and is **not** a radius token
//!
//! `--radius-sm` is 6, `--radius-md` is 8, `--radius-lg` is 10. The box's corner
//! is an *arbitrary value* — `.25rem` — so it is spelled as a rem multiple of the
//! 16px root here rather than read from a token that does not carry it. The
//! `::before` overlay's `rounded-[3px]` is a *different* arbitrary value again,
//! and the two are not derived from each other.

use gpui::{AnyElement, BoxShadow, Div, ParentElement as _, Pixels, Styled as _, div, px};

use crate::anchor::AnchorSink;
use crate::surfaces::rows::git_status_row::Breakpoint;
use crate::theme::{Color, Theme};

/// The only anchor on this surface: the
/// `<button role="checkbox" data-slot="checkbox">` box.
///
/// It carries the background `selected` moves, the 1px border, and the 4px
/// corner. See the module docs for why the indicator beneath it carries none.
pub const ID_CHECKBOX: &str = "checkbox";

/// The anchors on this surface whose boxes size to their own text
/// (`native/oracle/ANCHORS.md` v1.5).
///
/// **None.** `size-4.5 sm:size-4` authors both axes, and the box contains no text
/// node — an `<svg>` has element children, not text.
pub const CONTENT_SIZED: [&str; 0] = [];

/// The anchors on this surface whose **box height is their own line box**
/// (`native/oracle/ANCHORS.md` v1.6).
///
/// **None.** The box paints no text, so neither side emits a `font` and v1.6's
/// comparison has nothing on either end.
pub const LINE_SIZED: [&str; 0] = [];

/// `--spacing`, Tailwind's stock `0.25rem` at a 16px root.
///
/// Spelled once, as every other surface in this port spells it. `theme.css` does
/// **not** redefine it.
const SPACING: f32 = 4.0;

/// The root font size, for the two `rem` lengths this component authors as
/// arbitrary values rather than as utilities.
const ROOT_FONT_SIZE: f32 = 16.0;

/// `rounded-[.25rem]` on the box **and** on the indicator — 4px.
///
/// An *arbitrary value*, not a token: `--radius-sm` is 6, `--radius-md` 8 and
/// `--radius-lg` 10, and none of them is this. Derived from the rem so a project
/// that moved the root size moves it, rather than being written as `px(4.0)`.
pub const RADIUS: Pixels = px(0.25 * ROOT_FONT_SIZE);

/// `before:rounded-[3px]` on the `::before` overlay — a **different** arbitrary
/// value from [`RADIUS`], and not derived from it.
///
/// `input`'s overlay is one pixel rounder in than its control and says so as
/// arithmetic; this one is authored as a flat `3px` while the host is `.25rem`.
/// They happen to differ by a pixel too, and writing that as a derivation here
/// would invent a relationship the class list does not have.
pub const OVERLAY_RADIUS: Pixels = px(3.0);

/// `border` on the box — **a real 1px border on every cell**.
///
/// `button`'s and `input`'s trap, third occurrence. `ANCHORS.md` v1.1 compares
/// `border.w` *exactly*, and the live box reports `borderTopWidth: "1px"` in both
/// states. The variants change only the colour; there is no state in which this
/// becomes zero.
pub const BORDER_WIDTH: Pixels = px(1.0);

/// `focus-visible:ring-2` — **a box-shadow**, not a border.
///
/// The `ring-1` trap on the same component as the border trap, which is what
/// makes this one easy to get backwards. A port that reached for `.border_3()`
/// here would report `border.w: 3` against the reference's `1`.
pub const RING_WIDTH: Pixels = px(2.0);

/// `focus-visible:ring-offset-1`.
pub const RING_OFFSET: Pixels = px(1.0);

/// `dark:not-data-checked:bg-input/32` — **the one rule that carries `selected`**.
///
/// The same `/32` mix `input`'s control takes, and the same token. It is a
/// `dark:` variant, which is why this surface's `selected` cell is real in the
/// dark table and vacuous in the light one.
pub const DARK_UNCHECKED_ALPHA: f32 = 32.0;

/// `aria-invalid:border-destructive/36`.
pub const INVALID_BORDER_ALPHA: f32 = 36.0;

/// `focus-visible:aria-invalid:border-destructive/64`.
pub const INVALID_FOCUS_BORDER_ALPHA: f32 = 64.0;

/// `focus-visible:aria-invalid:ring-destructive/48`.
pub const INVALID_RING_ALPHA: f32 = 48.0;

/// `dark:aria-invalid:ring-destructive/24`.
///
/// `input`'s shape exactly, and read off the compiled sheet the same way: equal
/// specificity to [`INVALID_RING_ALPHA`]'s rule, and Tailwind emits the `dark:`
/// variant later, so **in the dark table this one wins**.
pub const INVALID_RING_ALPHA_DARK: f32 = 24.0;

/// `data-disabled:opacity-64`.
///
/// **Invisible**: v1.7's `visible` term fires only at *zero*, and the contract
/// has no opacity field.
pub const DISABLED_OPACITY: f32 = 0.64;

/// `-inset-px` on the indicator — how far outside the box's **padding** box it
/// sits on every side.
///
/// One pixel, and the same pixel [`BORDER_WIDTH`] is, which is exactly why the
/// fill lands on the box's *border* box: the padding box is inset by the border
/// all round, and a negative inset of the same size puts it back. Measured live
/// at `(0, 0, 16, 16)` relative to the root.
pub const INDICATOR_INSET: Pixels = BORDER_WIDTH;

/// Whether a `dark:` Tailwind variant is in force.
///
/// A local copy of `dropdown_menu`'s, `git_status_row`'s and `input`'s,
/// deliberately: the components are ported independently and a shared helper
/// would make one surface's diff reach into another's file. **Load-bearing here**
/// — it is what decides whether `selected` moves a recorded field at all.
fn is_dark(theme: &Theme) -> bool {
    *theme == Theme::DARK
}

/// The three states base-ui's `Checkbox` has, which are not two.
///
/// `indeterminate` is a real third value rather than a flavour of checked: it
/// takes the *same* `data-checked:bg-primary` fill but a different glyph and a
/// different `text-*` colour (`data-indeterminate:text-foreground` against the
/// indicator's own `text-primary-foreground`). Since the glyph is a box with no
/// path here, the only thing it moves that this port paints is that colour — and
/// the colour is on an unanchored element, so it moves nothing recorded.
///
/// **No live call site passes it.** All four `<Checkbox` call sites pass a plain
/// boolean `checked`; `indeterminate` is base-ui vocabulary the app never spends.
#[derive(Clone, Copy, Debug, Default, PartialEq, Eq)]
pub enum Checked {
    /// `data-unchecked` — the resting cell, and the one the live reference was
    /// captured from.
    #[default]
    Off,
    /// `data-checked`.
    On,
    /// `data-indeterminate`. **No reference**: no `<Checkbox` in `web/src/`
    /// passes it.
    Indeterminate,
}

/// Every checked state, for the tests and the usage line.
pub const ALL_CHECKED: [Checked; 3] = [Checked::Off, Checked::On, Checked::Indeterminate];

impl Checked {
    /// Its word on the command line and in a caption.
    #[must_use]
    pub const fn name(self) -> &'static str {
        match self {
            Self::Off => "off",
            Self::On => "on",
            Self::Indeterminate => "indeterminate",
        }
    }

    /// Whether the indicator is painted at all — `data-unchecked:hidden`.
    ///
    /// This is the predicate the module docs turn on: `false` here is
    /// `display: none` in the original, which the DOM extractor emits as a
    /// zero-box anchor and the driver omits entirely.
    #[must_use]
    pub const fn shows_indicator(self) -> bool {
        match self {
            Self::Off => false,
            Self::On | Self::Indeterminate => true,
        }
    }

    /// Whether a live `<Checkbox` call site can reach it.
    #[must_use]
    pub const fn live(self) -> bool {
        match self {
            Self::Off | Self::On => true,
            Self::Indeterminate => false,
        }
    }
}

/// `size` on the box: `size-4.5 sm:size-4`.
///
/// Authored by the **primitive** on both sides of the breakpoint, so — unlike
/// `badge`, where a call site's unprefixed `h-4` lost to the variant's `sm:h-4.5`
/// — there is no call site in the merge at all. None of the four live call sites
/// passes a `className`.
#[must_use]
pub const fn extent(breakpoint: Breakpoint) -> Pixels {
    px(SPACING
        * match breakpoint {
            Breakpoint::Base => 4.5,
            Breakpoint::Sm => 4.0,
        })
}

/// The tick's box: `size-3.5 sm:size-3`.
///
/// One `--spacing` step below the host at `sm:` and one at base, which is the
/// `SM_STEP_DROP` shape `input` writes out — but here the drop is on a different
/// number and the two are read separately rather than shared.
#[must_use]
pub const fn glyph_extent(breakpoint: Breakpoint) -> Pixels {
    px(SPACING
        * match breakpoint {
            Breakpoint::Base => 3.5,
            Breakpoint::Sm => 3.0,
        })
}

/// One `Checkbox`: the box, its overlay, and — when checked — the fill and the
/// tick.
#[derive(Clone, Copy, Debug, Default, PartialEq, Eq)]
pub struct Checkbox {
    /// Which side of `sm:` (640px) the **viewport** is on.
    pub breakpoint: Breakpoint,
    /// `checked` — §8.3's `selected`.
    ///
    /// **A parameter rather than a gpui refinement**, per `ANCHORS.md` §6.
    pub checked: Checked,
    /// The `disabled` prop. Live — the commit popover passes it while committing
    /// — and invisible: `opacity-64` and `shadow-none` have no field.
    pub disabled: bool,
    /// `:focus-visible`. Real in the class list, unreachable in practice, and
    /// invisible either way — the ring is a box-shadow. **It does move a compared
    /// field when combined with `invalid`**, through
    /// `focus-visible:aria-invalid:border-destructive/64`.
    pub focused: bool,
    /// `aria-invalid`. **Real, and it moves a compared field** — the box's
    /// `border.color`. **No reference**: no `<Checkbox` in `web/src/` passes it,
    /// exactly as no `<Input` does.
    pub invalid: bool,
}

impl Checkbox {
    /// The live commit-popover checkbox, as
    /// `web/src/features/git/components/commit-popover.tsx` renders it: one per
    /// changed file, no `className`, at rest, above the `sm:` breakpoint.
    ///
    /// **This call site rather than one of the other three**, because it is the
    /// one the reference was captured from and the only one reachable without a
    /// repository import. Its root's subtree holds no other surface's anchors, so
    /// `ANCHORS.md` v1.8 is satisfied without a declaration.
    ///
    /// `Checked::Off` is the resting cell. Unlike `switch`, the two states were
    /// **not** simultaneously live — every checkbox in the list starts checked —
    /// so the resting reference was produced by clicking one off and the selected
    /// one by clicking it back on, both settled before capture.
    #[must_use]
    pub fn fixture() -> Self {
        Self {
            breakpoint: Breakpoint::Sm,
            checked: Checked::Off,
            disabled: false,
            focused: false,
            invalid: false,
        }
    }

    /// The box's extent at this cell's breakpoint.
    #[must_use]
    pub const fn extent(self) -> Pixels {
        extent(self.breakpoint)
    }

    /// The tick's extent at this cell's breakpoint.
    #[must_use]
    pub const fn glyph_extent(self) -> Pixels {
        glyph_extent(self.breakpoint)
    }

    /// The box's background — **the one recorded field `selected` moves, and only
    /// in the dark table**.
    ///
    /// `bg-background`, with `dark:not-data-checked:bg-input/32` overriding it for
    /// an *unchecked* box in dark. So:
    ///
    /// | | light | dark |
    /// |---|---|---|
    /// | off | `background` | `input/32` |
    /// | on | `background` | `background` |
    ///
    /// The light column is the same colour twice, which is why `--flags selected`
    /// cannot fail on a light cell of this surface. Stated in
    /// [`Checkbox::selected_moves_a_recorded_field`] as a predicate rather than
    /// left in prose, so the surface's caption can read it rather than repeat it.
    #[must_use]
    pub fn background(&self, theme: &Theme) -> Color {
        if is_dark(theme) && !self.checked.shows_indicator() {
            theme.input.mix(DARK_UNCHECKED_ALPHA, Color::TRANSPARENT)
        } else {
            theme.background
        }
    }

    /// Whether `selected` moves any field the differ records, in this theme.
    ///
    /// **False in the light table.** The honest answer to "does this cell prove
    /// anything", exported so the surface's caption is a reading of the component
    /// rather than a second claim that could drift from it.
    #[must_use]
    pub fn selected_moves_a_recorded_field(theme: &Theme) -> bool {
        is_dark(theme)
    }

    /// The box's border colour, resolved through the variant chain.
    ///
    /// `border-input`, with `aria-invalid:border-destructive/36` over it and
    /// `focus-visible:aria-invalid:border-destructive/64` over that. **Focus alone
    /// moves nothing here** — unlike `input`, which carries a bare
    /// `has-focus-visible:border-ring`; `checkbox.tsx` has no such rule, and its
    /// focus ring is entirely a shadow.
    #[must_use]
    pub fn border_color(&self, theme: &Theme) -> Color {
        match (self.invalid, self.focused) {
            (true, true) => theme
                .destructive
                .mix(INVALID_FOCUS_BORDER_ALPHA, Color::TRANSPARENT),
            (true, false) => theme
                .destructive
                .mix(INVALID_BORDER_ALPHA, Color::TRANSPARENT),
            (false, _) => theme.input,
        }
    }

    /// The ring's colour, which paints only while [`Checkbox::focused`].
    ///
    /// **Invisible on every cell** — a box-shadow, `ANCHORS.md` §6.
    #[must_use]
    pub fn ring_color(&self, theme: &Theme) -> Color {
        if !self.invalid {
            return theme.ring;
        }
        let alpha = if is_dark(theme) {
            INVALID_RING_ALPHA_DARK
        } else {
            INVALID_RING_ALPHA
        };
        theme.destructive.mix(alpha, Color::TRANSPARENT)
    }

    /// The indicator's fill: `data-checked:bg-primary`.
    ///
    /// The same token the switch's checked track takes. **Painted and not
    /// recorded** — see the module docs.
    #[must_use]
    pub fn indicator_background(theme: &Theme) -> Color {
        theme.primary
    }

    /// The tick's colour: `text-primary-foreground`, or
    /// `data-indeterminate:text-foreground` for the dash.
    ///
    /// Painted and not recorded twice over: the element is unanchored, **and** an
    /// `<svg>` emits no `fg` even where it is anchored, because the extractor
    /// builds the text group from text nodes and an `<svg>` has none.
    #[must_use]
    pub fn glyph_color(&self, theme: &Theme) -> Color {
        match self.checked {
            Checked::Indeterminate => theme.foreground,
            Checked::Off | Checked::On => theme.primary_foreground,
        }
    }

    /// Whether `shadow-xs/5` survives.
    ///
    /// `[[data-disabled],[data-checked],[aria-invalid]]:shadow-none` drops it —
    /// and note that **checked** is in that list where `input`'s equivalent has
    /// focus instead. So a checked box has no shadow and a focused one still does.
    /// None of it is visible to the differ either way.
    #[must_use]
    pub const fn has_shadow(&self) -> bool {
        !(self.disabled || self.checked.shows_indicator() || self.invalid)
    }

    /// Renders the box and everything inside it, opting **only the box** into
    /// `anchors`.
    #[must_use]
    pub fn render(&self, theme: &Theme, anchors: &dyn AnchorSink) -> AnyElement {
        let mut element = self.host(theme).child(Self::overlay());
        if self.checked.shows_indicator() {
            element = element.child(self.indicator(theme));
        }
        anchors.root(ID_CHECKBOX.into(), element)
    }

    /// The box's own element — every recorded property on this surface.
    fn host(self, theme: &Theme) -> Div {
        let mut element = div()
            .relative()
            .flex()
            .flex_shrink_0()
            .items_center()
            .justify_center()
            .w(self.extent())
            .h(self.extent())
            .rounded(RADIUS)
            .border(BORDER_WIDTH)
            .border_color(self.border_color(theme).value())
            .bg(self.background(theme));

        if self.has_shadow() {
            // `shadow-xs/5`, the same preset `input`'s control takes and the same
            // reading: gpui's is byte-identical to what the app compiles, so the
            // literal lives inside gpui rather than in a component and
            // `check-invariants.sh` rule 4 stays satisfied.
            element = element.shadow_xs();
        }
        if self.focused {
            element = ring(element, self.ring_color(theme), theme);
        }
        if self.disabled {
            element = element.opacity(DISABLED_OPACITY);
        }
        element
    }

    /// The `::before` overlay: `absolute inset-0 rounded-[3px]`.
    ///
    /// **Deliberately unanchored**, for `input`'s reason rather than
    /// `resizable`'s: §3's pseudo-backed shortcut is *legal* here — the pseudo
    /// really is `position:absolute; inset:0` — and taking it would still be
    /// wrong, because a pseudo-backed anchor **replaces** the host's record. It
    /// would throw away the box's own background, its 1px border and its 4px
    /// corner, which are the entire surface.
    ///
    /// Its two inset shadows are **not painted**:
    /// `before:shadow-[0_1px_--theme(--color-black/4%)]` and the dark
    /// `[0_-1px_--theme(--color-white/6%)]` need Tailwind's own black and white,
    /// which rule 4 will not let a component mint — the same wall `input`'s
    /// overlay meets, recorded there in the same words.
    fn overlay() -> Div {
        div()
            .absolute()
            .top(px(0.0))
            .right(px(0.0))
            .bottom(px(0.0))
            .left(px(0.0))
            .rounded(OVERLAY_RADIUS)
    }

    /// The fill and the tick: `absolute -inset-px … data-checked:bg-primary`.
    ///
    /// Rendered only while [`Checked::shows_indicator`], which is the original's
    /// `data-unchecked:hidden` — and, per the module docs, the reason this element
    /// carries no anchor.
    ///
    /// The negative inset is against the host's **padding** box, which is inset by
    /// the border all round, so a `-1px` on each side puts the fill exactly on the
    /// host's border box. Measured live at `(0, 0, 16, 16)` relative to the root.
    fn indicator(self, theme: &Theme) -> Div {
        div()
            .absolute()
            .top(-INDICATOR_INSET)
            .left(-INDICATOR_INSET)
            .flex()
            .items_center()
            .justify_center()
            .w(self.extent())
            .h(self.extent())
            .rounded(RADIUS)
            .bg(Self::indicator_background(theme))
            .text_color(self.glyph_color(theme))
            .child(self.glyph())
    }

    /// The `<svg>` tick's **box**, with no path.
    ///
    /// P3.8's house style, stated in `crowbar_mark` and `sidebar_toggle_icon`: an
    /// `<svg>`'s `stroke`, `stroke-width`, `stroke-linecap` and `d` have no field
    /// in the contract, so the port draws the box the glyph occupies and not the
    /// glyph. Measured live at `12 × 12` at `(2, 2)` relative to the root, with
    /// `stroke: oklch(0.98 0.027 98)` and `stroke-width: 3px` — neither of which
    /// any snapshot can carry.
    ///
    /// The checked tick and the indeterminate dash are the **same box**; only
    /// [`Checkbox::glyph_color`] differs, and that is on an unanchored element.
    fn glyph(self) -> Div {
        div()
            .flex_shrink_0()
            .w(self.glyph_extent())
            .h(self.glyph_extent())
    }
}

/// Adds `focus-visible:ring-2 ring-offset-1`'s two shadow layers in front of
/// whatever shadow `element` already carries.
///
/// A local copy of the one every other ported surface carries, for their reason:
/// the components are ported independently and a shared helper would make one
/// surface's diff reach into another's file.
fn ring(mut element: Div, color: Color, theme: &Theme) -> Div {
    // `ring-offset-background`, beside a real `ring-offset-1`, so the offset
    // layer paints — the same arrangement `switch` has and `input` does not.
    let offset =
        BoxShadow::new(px(0.0), px(0.0), theme.background.value()).spread_radius(RING_OFFSET);
    let halo =
        BoxShadow::new(px(0.0), px(0.0), color.value()).spread_radius(RING_OFFSET + RING_WIDTH);
    let shadows = element.style().box_shadow.get_or_insert_with(Vec::new);
    shadows.insert(0, halo);
    shadows.insert(0, offset);
    element
}

#[cfg(test)]
mod tests {
    use super::{
        ALL_CHECKED, BORDER_WIDTH, CONTENT_SIZED, Checkbox, Checked, DARK_UNCHECKED_ALPHA,
        DISABLED_OPACITY, ID_CHECKBOX, INDICATOR_INSET, INVALID_BORDER_ALPHA,
        INVALID_FOCUS_BORDER_ALPHA, INVALID_RING_ALPHA, INVALID_RING_ALPHA_DARK, LINE_SIZED,
        OVERLAY_RADIUS, RADIUS, RING_OFFSET, RING_WIDTH, extent, glyph_extent,
    };
    use crate::surfaces::rows::git_status_row::Breakpoint;
    use crate::theme::{Color, Theme};
    use gpui::px;

    /// The fixture is the live commit-popover checkbox, and the resting cell is
    /// the **unchecked** one.
    #[test]
    fn the_fixture_is_the_live_commit_popover_checkbox() {
        let checkbox = Checkbox::fixture();

        assert_eq!(checkbox.breakpoint, Breakpoint::Sm);
        assert_eq!(checkbox.checked, Checked::Off);
        assert!(!checkbox.disabled);
        assert!(!checkbox.focused);
        assert!(!checkbox.invalid);

        // The live numbers, from `/tmp/p3-ref-checkbox.json`.
        assert_eq!(checkbox.extent(), px(16.0));
        assert_eq!(RADIUS, px(4.0));
        assert_eq!(BORDER_WIDTH, px(1.0));
        assert_eq!(checkbox.glyph_extent(), px(12.0));
    }

    /// **`selected` moves exactly one recorded field, and only in dark.**
    ///
    /// The assertion the whole item turns on for this component, and the half a
    /// passing run could otherwise hide: in the light table the two cells are the
    /// same colour, so `--flags selected` there cannot fail.
    #[test]
    fn selected_moves_the_background_in_dark_and_nothing_at_all_in_light() {
        let off = Checkbox::fixture();
        let on = Checkbox {
            checked: Checked::On,
            ..Checkbox::fixture()
        };

        // Dark: the one rule that carries it, `dark:not-data-checked:bg-input/32`.
        assert_eq!(
            off.background(&Theme::DARK),
            Theme::DARK
                .input
                .mix(DARK_UNCHECKED_ALPHA, Color::TRANSPARENT),
        );
        assert_eq!(on.background(&Theme::DARK), Theme::DARK.background);
        assert_ne!(off.background(&Theme::DARK), on.background(&Theme::DARK));
        assert!(Checkbox::selected_moves_a_recorded_field(&Theme::DARK));

        // Light: the same colour twice. This cell cannot fail.
        assert_eq!(off.background(&Theme::LIGHT), Theme::LIGHT.background);
        assert_eq!(on.background(&Theme::LIGHT), Theme::LIGHT.background);
        assert_eq!(off.background(&Theme::LIGHT), on.background(&Theme::LIGHT));
        assert!(!Checkbox::selected_moves_a_recorded_field(&Theme::LIGHT));

        // And nothing else moves in *either* theme — every other recorded field
        // is identical, which is why the claim above is "exactly one".
        for theme in [Theme::LIGHT, Theme::DARK] {
            assert_eq!(off.extent(), on.extent());
            assert_eq!(off.border_color(&theme), on.border_color(&theme));
            assert_eq!(off.border_color(&theme), theme.input);
        }
    }

    /// The mix is a real third colour, or the `/32` would be decoration.
    #[test]
    fn the_dark_unchecked_mix_is_neither_of_the_tokens_it_is_made_from() {
        let off = Checkbox::fixture();
        assert_ne!(off.background(&Theme::DARK), Theme::DARK.input);
        assert_ne!(off.background(&Theme::DARK), Theme::DARK.background);
        assert!((DARK_UNCHECKED_ALPHA - 32.0).abs() < f32::EPSILON);
    }

    /// **`indeterminate` takes the checked fill**, so it is not a third geometry
    /// — and no live call site reaches it.
    #[test]
    fn indeterminate_is_the_checked_fill_with_another_glyph_colour() {
        let on = Checkbox {
            checked: Checked::On,
            ..Checkbox::fixture()
        };
        let mixed = Checkbox {
            checked: Checked::Indeterminate,
            ..Checkbox::fixture()
        };

        assert!(mixed.checked.shows_indicator());
        for theme in [Theme::LIGHT, Theme::DARK] {
            // Same recorded background as checked — the box cannot tell them apart.
            assert_eq!(mixed.background(&theme), on.background(&theme));
            // Different glyph colour, on an element that carries no anchor.
            assert_eq!(mixed.glyph_color(&theme), theme.foreground);
            assert_eq!(on.glyph_color(&theme), theme.primary_foreground);
            assert_ne!(mixed.glyph_color(&theme), on.glyph_color(&theme));
        }
        // Same box, so the glyph is one geometry with two colours.
        assert_eq!(mixed.glyph_extent(), on.glyph_extent());

        assert!(!Checked::Indeterminate.live());
        assert!(Checked::Off.live());
        assert!(Checked::On.live());
        assert_eq!(
            ALL_CHECKED.map(Checked::name),
            ["off", "on", "indeterminate"]
        );
        assert_eq!(Checked::default(), Checked::Off);
    }

    /// **`data-unchecked:hidden` is the predicate the anchor set turns on.**
    ///
    /// The resting cell paints no indicator at all, which is `display: none` in
    /// the original — the case the two extractors disagree about and the reason
    /// the fill carries no anchor. See the module docs.
    #[test]
    fn the_resting_cell_paints_no_indicator() {
        assert!(!Checked::Off.shows_indicator());
        assert!(Checked::On.shows_indicator());
        assert!(Checked::Indeterminate.shows_indicator());
        assert!(!Checkbox::fixture().checked.shows_indicator());
    }

    /// The two `sm:` steps, and the control that they are genuinely different
    /// pictures.
    #[test]
    fn both_boxes_follow_the_viewport_across_the_breakpoint() {
        const STEP: f32 = 4.0;

        assert_eq!(extent(Breakpoint::Base), px(STEP * 4.5));
        assert_eq!(extent(Breakpoint::Sm), px(STEP * 4.0));
        assert_eq!(glyph_extent(Breakpoint::Base), px(STEP * 3.5));
        assert_eq!(glyph_extent(Breakpoint::Sm), px(STEP * 3.0));

        assert_ne!(extent(Breakpoint::Base), extent(Breakpoint::Sm));
        assert_ne!(glyph_extent(Breakpoint::Base), glyph_extent(Breakpoint::Sm));

        // The glyph is one step inside the box at both breakpoints, which is what
        // centres it at `(2, 2)`.
        for breakpoint in [Breakpoint::Base, Breakpoint::Sm] {
            assert_eq!(
                extent(breakpoint) - glyph_extent(breakpoint),
                px(STEP),
                "{breakpoint:?}",
            );
        }
    }

    /// **The border chain, in the order the compiled sheet emits it** — and the
    /// place this component differs from `input`: **focus alone moves nothing**.
    #[test]
    fn focus_alone_does_not_move_the_border_but_invalid_does() {
        for theme in [Theme::LIGHT, Theme::DARK] {
            let resting = Checkbox::fixture();
            assert_eq!(resting.border_color(&theme), theme.input);

            // `checkbox.tsx` has no bare `focus-visible:border-*`, unlike
            // `input.tsx`'s `has-focus-visible:border-ring`.
            let focused = Checkbox {
                focused: true,
                ..Checkbox::fixture()
            };
            assert_eq!(focused.border_color(&theme), theme.input);
            assert_eq!(focused.border_color(&theme), resting.border_color(&theme));

            let invalid = Checkbox {
                invalid: true,
                ..Checkbox::fixture()
            };
            assert_eq!(
                invalid.border_color(&theme),
                theme
                    .destructive
                    .mix(INVALID_BORDER_ALPHA, Color::TRANSPARENT),
            );
            assert_ne!(invalid.border_color(&theme), resting.border_color(&theme));

            // And the doubly-qualified rule beats it.
            let both = Checkbox {
                focused: true,
                invalid: true,
                ..Checkbox::fixture()
            };
            assert_eq!(
                both.border_color(&theme),
                theme
                    .destructive
                    .mix(INVALID_FOCUS_BORDER_ALPHA, Color::TRANSPARENT),
            );
            assert_ne!(both.border_color(&theme), invalid.border_color(&theme));
        }
    }

    /// The ring's colour, including the `dark:` rule that **outranks** the one
    /// with more qualifiers — `input`'s finding, on a second component.
    #[test]
    fn the_dark_invalid_ring_beats_the_focused_invalid_one() {
        let invalid = Checkbox {
            focused: true,
            invalid: true,
            ..Checkbox::fixture()
        };

        assert_eq!(
            invalid.ring_color(&Theme::LIGHT),
            Theme::LIGHT
                .destructive
                .mix(INVALID_RING_ALPHA, Color::TRANSPARENT),
        );
        assert_eq!(
            invalid.ring_color(&Theme::DARK),
            Theme::DARK
                .destructive
                .mix(INVALID_RING_ALPHA_DARK, Color::TRANSPARENT),
        );
        assert!((INVALID_RING_ALPHA - INVALID_RING_ALPHA_DARK).abs() > f32::EPSILON);

        for theme in [Theme::LIGHT, Theme::DARK] {
            let focused = Checkbox {
                focused: true,
                ..Checkbox::fixture()
            };
            assert_eq!(focused.ring_color(&theme), theme.ring);
        }
    }

    /// **`checked` drops the shadow where `input`'s equivalent drops it on
    /// focus** — the same class shape with a different member list, which is
    /// exactly the sort of thing a port copies wrong.
    #[test]
    fn a_checked_box_has_no_shadow_and_a_focused_one_still_does() {
        assert!(Checkbox::fixture().has_shadow());

        for state in [
            Checkbox {
                checked: Checked::On,
                ..Checkbox::fixture()
            },
            Checkbox {
                checked: Checked::Indeterminate,
                ..Checkbox::fixture()
            },
            Checkbox {
                disabled: true,
                ..Checkbox::fixture()
            },
            Checkbox {
                invalid: true,
                ..Checkbox::fixture()
            },
        ] {
            assert!(!state.has_shadow(), "{state:?}");
        }

        // Focus is **not** in the list, unlike `input`'s.
        assert!(
            Checkbox {
                focused: true,
                ..Checkbox::fixture()
            }
            .has_shadow(),
        );

        assert!((DISABLED_OPACITY - 0.64).abs() < f32::EPSILON);
        const { assert!(DISABLED_OPACITY > 0.0) };
    }

    /// **The indicator's negative inset is the border width**, which is what puts
    /// the fill on the host's border box rather than a pixel inside it.
    #[test]
    fn the_indicators_inset_is_the_border_it_cancels() {
        assert_eq!(INDICATOR_INSET, BORDER_WIDTH);
        assert_eq!(INDICATOR_INSET, px(1.0));

        // Padding box + two insets = border box, at both breakpoints.
        for breakpoint in [Breakpoint::Base, Breakpoint::Sm] {
            let padding_box = extent(breakpoint) - BORDER_WIDTH * 2.0;
            assert_eq!(padding_box + INDICATOR_INSET * 2.0, extent(breakpoint));
        }
    }

    /// **The two radii are different arbitrary values and neither is a token.**
    #[test]
    fn the_corner_is_an_arbitrary_value_and_the_overlays_is_a_different_one() {
        assert_eq!(RADIUS, px(4.0));
        assert_eq!(OVERLAY_RADIUS, px(3.0));
        assert_ne!(RADIUS, OVERLAY_RADIUS);

        for theme in [Theme::LIGHT, Theme::DARK] {
            assert_ne!(RADIUS, theme.radius_sm.value());
            assert_ne!(RADIUS, theme.radius_md.value());
            assert_ne!(RADIUS, theme.radius_lg.value());
        }
    }

    /// `border` is 1px and the ring is not it — both traps, on one component.
    #[test]
    fn the_border_is_one_pixel_and_the_ring_is_a_shadow() {
        assert_eq!(BORDER_WIDTH, px(1.0));
        assert_eq!(RING_WIDTH, px(2.0));
        assert_eq!(RING_OFFSET, px(1.0));
        assert_ne!(RING_WIDTH, BORDER_WIDTH);
    }

    /// One anchor, named for the `data-slot`, and it is the root.
    #[test]
    fn the_surface_has_one_anchor_and_declares_neither_sizing() {
        assert_eq!(ID_CHECKBOX, "checkbox");
        assert_eq!(CONTENT_SIZED, [] as [&str; 0]);
        assert_eq!(LINE_SIZED, [] as [&str; 0]);
    }
}
