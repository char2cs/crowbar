//! `button` — the app's most-used primitive, and the first ported component
//! whose **border is one pixel wide in every cell**.
//!
//! The native half of `web/src/components/ui/button.tsx`, whose class lists live
//! next door in `web/src/components/ui/button-variants.ts` as a `cva()` with a
//! base string, seven variants and ten sizes. Every value below came out of the
//! app's own `tailwindcss` 4.3.0 with the utility as a candidate — the method
//! `native/MAPPING.md` fixes — and the ones that are not Tailwind's stock values
//! are named as such. See `native/mapping/button.md`.
//!
//! # `border` is the headline, and it is the opposite of every earlier trap
//!
//! `native/MAPPING.md` records twice that **`ring-1` is a box-shadow, not a
//! border**, and that a port reaching for `.border_1()` reports `border.w: 1`
//! against a reference's `0` — the one field `ANCHORS.md` v1.1 compares
//! *exactly*. This component is the mirror image. Its base class list carries a
//! bare `border`, which compiles to `border-width: 1px` **unconditionally**, and
//! the variants only ever change the *colour* — `border-transparent` on `ghost`,
//! `link` and `secondary`. So:
//!
//! * every anchored button reports `border.w: 1`, in every variant and every
//!   cell, measured on the live app (`ghost`: `borderTopWidth: "1px"`,
//!   `borderTopColor: "rgba(0, 0, 0, 0)"`);
//! * a port that skipped [`BORDER_WIDTH`] because "a ghost button has no visible
//!   border" would report `0` against `1` on **every cell of the matrix**;
//! * and the pixel is not decorative — `box-sizing: border-box` is on, so it eats
//!   into the padding box and moves the `::before` overlay and the icon.
//!
//! # What is visible to the differ, per axis
//!
//! | Axis | Here |
//! |---|---|
//! | `--width` | **weak, but not for the reason this row used to give.** Nine of the ten sizes author their own box; only the five non-icon sizes take their width from their content — and that path is exercised in the product, not never: **at least 72** live, non-test `web/src/` call sites render a `<Button>` with *visible* text (counted by parsing the JSX, not by a regex — a naive `<Button\b(.*?)>` walks into an arrow body's `>` and lies; `developer-settings.tsx:178`'s "Export settings" is one). That 72 is a floor, not a point estimate: it excludes every case that took a judgment call to classify — e.g. `code-block-node.tsx:391`'s icon button, whose only child text is an `sr-only` span and so isn't *visible*, and a couple of icon-plus-glyph buttons where "label" itself is debatable. None of the 72 is *this surface's own* reference, though: the fixture workspace's nine captured `[data-slot=button]` elements are all icon-only (`native/mapping/button.md` §9), so the content-sized path stays real in the app and unexercised by this differ specifically |
//! | `--viewport-width` | **real, and the strongest axis this surface has.** Every one of the ten sizes carries an `sm:` variant that changes its box, and two of them change the type scale as well |
//! | `--theme` | **real** for six of the seven variants — `bg`, `fg` and `border.color` all move. Vacuous on `link`, which paints no background and no border colour |
//! | `--content` | real **only with `--label`**, which is off by default because the reference's nine buttons are all icon-only |
//! | `hover` | a **real, visible** state — the first in the port. `hover:bg-*` changes `bg` on six of seven variants. It has **no reference**: synthetic pointer events are denied on this project's machines |
//! | `focus` | a real state with **no field**. `focus-visible:ring-2 ring-offset-1` is two box-shadows, and `ANCHORS.md` §6 has none |
//! | `selected` → the `active` prop | a real rule (`bg-accent/20`) that **no live call site passes** |
//!
//! # Why the two declaration lists are empty
//!
//! [`LINE_SIZED`] is empty because **every size authors its own height** —
//! `h-9`, `size-8`, `h-7` and the rest. That is the `git-row-badge` trap of
//! `ANCHORS.md` v1.6 in a new place: a button paints text, so it reads like the
//! case the rule was written for, and declaring it would compare a 32px box
//! against a 20px line box and manufacture a 12px delta on an anchor both
//! engines agree on.
//!
//! [`CONTENT_SIZED`] is empty for a different and less comfortable reason, and
//! it is stated rather than glossed. The five non-icon sizes author no width, so
//! a Button with a label **is** a content-sized box and v1.5 would apply. The
//! declaration has to be made on *both* sides, and the React side's spelling is a
//! `data-oracle-content-sized` attribute on `button.tsx` — which this item may
//! not add: its remit on that file is `data-oracle-id` and nothing else. Leaving
//! both sides undeclared is the safe direction rather than a blind spot: it can
//! manufacture a delta of at most the ceil excess (< 1px) and cannot hide one.
//! It does not fire today, but the reason this sentence used to give was false:
//! **at least 72** live, non-test call sites in `web/src/` render a `<Button>`
//! with visible text (see the `--width` row above for how that floor was
//! counted and what it deliberately excludes) — `developer-settings.tsx:178`
//! is one. Labelled Buttons are ordinary, not hand-built exceptions. What
//! actually keeps the rule quiet is narrower: none of them was *captured* for
//! this surface's own reference — the fixture workspace's nine
//! `[data-slot=button]` elements are all icon-only (`native/mapping/button.md`
//! §9). That is one differently-scoped capture away from changing, so it is
//! the bound above, not this absence, that makes leaving both sides
//! undeclared safe.

use gpui::{
    AnyElement, BoxShadow, Div, FontWeight, IntoElement as _, ParentElement as _, Pixels, Rems,
    SharedString, Styled as _, div, px, relative,
};

use super::anchor::{AnchorId, AnchorSink};
use super::git_status_row::Breakpoint;
use crate::theme::{Color, Theme};

/// The root anchor: the `<button>` itself. Every other bound on this surface is
/// reported relative to it (`native/oracle/ANCHORS.md` §4).
///
/// Written as a **per-slot default** in `button.tsx`, before `{...props}`, so a
/// call site can override it — the convention `dropdown-menu` set in P2.1. It
/// has to be overridable here more than anywhere: there are nine Buttons in one
/// live document, and `extractSnapshot` refuses a declared surface whose id
/// appears twice.
pub const ID_BUTTON: &str = "button";

/// The `<Spinner>` `button.tsx` renders when `loading` is set.
///
/// **No live call site passes `loading`** — 142 `<Button` elements in
/// `web/src/`, none of them with the prop — so this anchor has no reference at
/// all. The same shape of finding as `resizable`'s grip and Phase 1's
/// `git-row-dir`, and the reason the `button` surface drives it from its own
/// `--loading` option rather than from the §8.3 `loading` flag.
pub const ID_LOADING_INDICATOR: &str = "button-loading-indicator";

/// The anchors on this surface whose boxes size to their own text
/// (`native/oracle/ANCHORS.md` v1.5).
///
/// **Empty, and not because the case does not arise** — see the module docs. The
/// five non-icon sizes author no width, so a labelled button's used width *is*
/// its content's max-content width and v1.5 would apply. A v1.5 declaration has
/// to be made on both sides and the React spelling is an attribute this item may
/// not add to `button.tsx`, so neither side declares it and the differ compares
/// the raw widths, which is the strict direction.
///
/// An empty list rather than no list, for P2.1's reason: the React side's (also
/// empty) declaration set has to be diffable against this one.
pub const CONTENT_SIZED: [&str; 0] = [];

/// The anchors on this surface whose **box height is their own line box**
/// (`native/oracle/ANCHORS.md` v1.6).
///
/// **Empty, and here that is a measurement rather than an omission.** Every one
/// of the ten sizes authors a height — `h-9`/`sm:h-8`, `size-8`/`sm:size-7` and
/// the rest — so no button's box is derived from its line box. See
/// [`Size::extent`] and the v1.6 note in the module docs.
pub const LINE_SIZED: [&str; 0] = [];

/// `--spacing`, Tailwind's stock `0.25rem` at a 16px root.
///
/// `theme.css` does **not** redefine it — checked, because P2.1 found three
/// values in `dropdown-menu` that it *does* redefine. Spelled once so a theme
/// that moved the step would move every length below together.
const SPACING: f32 = 4.0;

/// The base class list's bare `border`, which compiles to `border-width: 1px`
/// with no variant able to change it.
///
/// The whole of the module docs' headline lives here. It is 1px on `ghost` and
/// `link` too, where the colour is `transparent` — and `ANCHORS.md` v1.3 fixed
/// that `border.color` is compared only when `w > 0`, so a transparent 1px
/// border is a *comparison* on both fields rather than an absence on either.
pub const BORDER_WIDTH: Pixels = px(1.0);

/// `focus-visible:ring-2` — `0 0 0 calc(2px + var(--tw-ring-offset-width))`, and
/// `focus-visible:ring-offset-1` sets that width to 1px. So the spread is **3**.
///
/// A **box-shadow**, not a border, exactly as `resizable`'s `focus-visible:ring-1`
/// is. Unlike `resizable`'s, this one has a live offset layer as well
/// ([`RING_OFFSET_WIDTH`]): `ring-offset-1` moves `--tw-ring-offset-width` off
/// its `0px` initial, so the offset shadow paints instead of staying at its
/// `0 0 #0000`. Both are invisible to the contract (`ANCHORS.md` §6).
pub const RING_SPREAD: Pixels = px(3.0);

/// `focus-visible:ring-offset-1` — a second shadow layer, `0 0 0 1px` in
/// `ring-offset-background`, painted **in front of** [`RING_SPREAD`]'s.
///
/// The order is Tailwind's own composite:
/// `var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)`.
pub const RING_OFFSET_WIDTH: Pixels = px(1.0);

/// `disabled:opacity-64`, which compiles to `opacity: 64%`.
///
/// **Painted and invisible.** `ANCHORS.md` has no opacity field, and v1.7's
/// opacity term makes `visible` false only at *zero* — so a disabled button is
/// still `visible: true` on both sides, and 64% is a difference neither
/// extractor can report. It is painted anyway, because it is what the reference
/// looks like.
pub const DISABLED_OPACITY: f32 = 0.64;

/// The `active` prop's `bg-accent/20`, appended by `cn(…)` **after**
/// `buttonVariants(…)` and therefore beating every variant's own `bg-*`.
///
/// It does *not* beat `hover:bg-*` or `data-pressed:bg-*`: those are different
/// tailwind-merge groups, so both survive the merge, and at runtime
/// `.hover\:bg-accent:hover` is one selector more specific than `.bg-accent\/20`.
/// So `active` replaces the **resting** background only.
pub const ACTIVE_TINT: f32 = 20.0;

/// `[&_svg]:-mx-0.5` — every icon inside a button pulls its neighbours in by
/// 2px on each side. Measured live: the reference's `<svg>` reports
/// `margin-left: -2px`.
///
/// # gpui cannot lay this out, and that is the largest finding in this port
///
/// **A negative inline margin on an in-flow flex item breaks taffy's
/// content-based main-size resolution.** Measured in the harness, isolated from
/// this component — a flex row with `px-11 border-1`, a 16px item and a text
/// run:
///
/// | item margin | container width | CSS |
/// |---|---|---|
/// | `0` | 77 | 77 |
/// | `+2` | 81 | 81 |
/// | `-1` | **24** | 75 |
/// | `-2` | **24** | 73 |
/// | `-4` | **24** | 69 |
///
/// 24 is the padding box and the two border pixels: the container's whole
/// content contribution is gone. It is not a clamp at zero either — with a wider
/// run the same tree gives 199 against CSS's 323 at `-2` and 71 at `-4`, so the
/// error scales with the margin and is not a constant. Positive margins are
/// exact. A container with an **authored** width is unaffected, which is why the
/// five square sizes and every earlier component in this port never met it:
/// `dropdown-menu`'s `-mx-1` separator and `sidebar-carousel`'s negative
/// percentage margin both sit in containers whose main size is already definite.
///
/// # What the port does: resolves it by hand, into the glyph's own box
///
/// [`Button::glyph`] renders a box of [`Size::glyph_box`] — the icon less the
/// two margins — and **no margin at all**. Every measurable quantity is then
/// exactly CSS's: the button's border box, the label's advance, the gap. What
/// changes is the glyph's own box, from 16 to 12, and that costs **nothing**:
/// the glyph is an *empty* box standing in for a call-site `<svg>` this port
/// does not draw, so 16px of nothing and 12px of nothing are the same picture —
/// and it is unanchorable on the reference side anyway, because `button.tsx`
/// cannot put an attribute on a child a call site passes.
///
/// [`Button::spinner`] **keeps** the margin, for two reasons that have to hold
/// together: it is `position: absolute`, so it contributes nothing to the
/// content size and cannot trip the defect, and it *is* anchored — so its box
/// has to be the reference's 16, not 12.
///
/// The alternative — pinning the button's width in the component — is the one
/// `native/oracle/ANCHORS.md` rejects by name, twice.
pub const ICON_MARGIN_X: Pixels = px(-SPACING * 0.5);

/// Tailwind's `text-lg`, which **no crowbar token carries**.
///
/// `native/MAPPING.md` records the `--ui-text-*` trade: Tailwind's `text-sm`,
/// `text-xs` and `text-base` are 0.875 / 0.75 / 1rem and `--ui-text-base`,
/// `--ui-text-sm` and `--ui-text-lg` are the same three numbers, so the port
/// reads the token that carries the number and says so. The trade runs out
/// here. `text-lg` is **1.125rem** and `--ui-text-xl` is **1.25rem** — the two
/// scales diverge at the fourth step, and reading `--ui-text-xl` would paint an
/// 20px button label where the reference paints 18.
///
/// So this is a literal, named, with the reason — the same statement
/// [`SPACING`] makes about `--spacing`. It is reachable only from `--size xl`
/// below the `sm` breakpoint, and `xl` is one of the four sizes **no live call
/// site uses**.
pub const TEXT_LG: Rems = Rems(1.125);

/// Tailwind's line-height for `text-base`: `calc(1.5 / 1)`.
const LINE_HEIGHT_BASE: f32 = 1.5;
/// Tailwind's line-height for `text-sm`: `calc(1.25 / 0.875)`.
const LINE_HEIGHT_SM: f32 = 1.25 / 0.875;
/// Tailwind's line-height for `text-xs`: `calc(1 / 0.75)`.
const LINE_HEIGHT_XS: f32 = 1.0 / 0.75;
/// Tailwind's line-height for `text-lg`: `calc(1.75 / 1.125)`.
const LINE_HEIGHT_LG: f32 = 1.75 / 1.125;

/// A type step: the size and the unitless line height Tailwind pairs with it.
#[derive(Clone, Copy, Debug, PartialEq)]
pub struct TypeStep {
    /// `font-size`.
    pub size: Rems,
    /// `line-height`, unitless — the ratio, not the resolved px, because gpui
    /// takes a `relative(…)` exactly as CSS takes a number.
    pub line_height: f32,
}

/// `cva`'s `size` union, all ten of them.
///
/// **Six are live and four are dead**, counted over every `<Button` in
/// `web/src/` (142 of them): `default` 82, `sm` 33, `xs` 11, `icon-sm` 7,
/// `icon` 6, `icon-xs` 3 — and **zero** for `lg`, `xl`, `icon-lg` and
/// `icon-xl`. The four dead ones are rendered anyway, for `resizable`'s reason
/// about the grip: they are real arms of the primitive, and a port that dropped
/// them would be describing a smaller component than the one being ported.
/// [`Size::live`] is what a caption uses to say which is which.
#[derive(Clone, Copy, Debug, Default, PartialEq, Eq)]
pub enum Size {
    /// `h-9 px-[calc(--spacing(3)-1px)] sm:h-8` — `cva`'s own default, and the
    /// most-used arm (82 of 142 call sites pass no `size` at all).
    #[default]
    Default,
    /// `h-8 gap-1.5 px-[calc(--spacing(2.5)-1px)] sm:h-7`.
    Sm,
    /// `h-10 px-[calc(--spacing(3.5)-1px)] sm:h-9`. **Dead.**
    Lg,
    /// `h-11 px-[calc(--spacing(4)-1px)] text-lg sm:h-10 sm:text-base`. **Dead**,
    /// and the only arm that reaches [`TEXT_LG`].
    Xl,
    /// `h-7 gap-1 rounded-md px-[calc(--spacing(2)-1px)] text-sm sm:h-6 sm:text-xs`.
    Xs,
    /// `size-9 sm:size-8`.
    Icon,
    /// `size-8 sm:size-7`.
    IconSm,
    /// `size-10 sm:size-9`. **Dead.**
    IconLg,
    /// `size-11 sm:size-10` plus its own bigger glyph. **Dead.**
    IconXl,
    /// `size-7 rounded-md sm:size-6` plus its own smaller glyph.
    IconXs,
}

/// Every size, in `cva`'s declaration order, so a caller cannot drive a subset
/// by accident and a `--help` line cannot advertise one that does not exist.
pub const ALL_SIZES: [Size; 10] = [
    Size::Default,
    Size::Icon,
    Size::IconLg,
    Size::IconSm,
    Size::IconXl,
    Size::IconXs,
    Size::Lg,
    Size::Sm,
    Size::Xl,
    Size::Xs,
];

impl Size {
    /// Its word on the command line and in a caption — `cva`'s own key.
    #[must_use]
    pub const fn name(self) -> &'static str {
        match self {
            Self::Default => "default",
            Self::Sm => "sm",
            Self::Lg => "lg",
            Self::Xl => "xl",
            Self::Xs => "xs",
            Self::Icon => "icon",
            Self::IconSm => "icon-sm",
            Self::IconLg => "icon-lg",
            Self::IconXl => "icon-xl",
            Self::IconXs => "icon-xs",
        }
    }

    /// Whether any live `<Button` in `web/src/` passes this size.
    ///
    /// Data rather than an opinion: counted over all 142 `<Button` elements.
    /// A dead size renders on the native side and the reference cannot produce
    /// it, so the caption says so per cell rather than letting a reader bank a
    /// comparison that has nothing on the other end.
    #[must_use]
    pub const fn live(self) -> bool {
        !matches!(self, Self::Lg | Self::Xl | Self::IconLg | Self::IconXl)
    }

    /// Whether this size authors **both** axes (`size-*`) rather than only the
    /// height (`h-*`).
    ///
    /// The five `icon*` arms do, which is what makes them square, gives them no
    /// horizontal padding, and takes them out of v1.5's reach.
    #[must_use]
    pub const fn square(self) -> bool {
        matches!(
            self,
            Self::Icon | Self::IconSm | Self::IconLg | Self::IconXl | Self::IconXs
        )
    }

    /// The authored box extent: `h-*` for a text size, `size-*` for an icon one.
    ///
    /// Ten sizes but **five numbers**: each text size shares its extent with its
    /// icon twin, because `h-9`/`size-9`, `h-8`/`size-8` and the rest are the
    /// same `--spacing` multiple. Read as `(base, sm)` — the `sm:` variant is
    /// always exactly one step smaller, which is the whole of what the
    /// breakpoint does to this component's box.
    #[must_use]
    pub fn extent(self, breakpoint: Breakpoint) -> Pixels {
        let (base, small) = match self {
            Self::Xl | Self::IconXl => (11.0, 10.0),
            Self::Lg | Self::IconLg => (10.0, 9.0),
            Self::Default | Self::Icon => (9.0, 8.0),
            Self::Sm | Self::IconSm => (8.0, 7.0),
            Self::Xs | Self::IconXs => (7.0, 6.0),
        };
        px(SPACING
            * match breakpoint {
                Breakpoint::Base => base,
                Breakpoint::Sm => small,
            })
    }

    /// `px-[calc(--spacing(n)-1px)]`, and **zero** on every icon size.
    ///
    /// The `-1px` is the base class list's `border` being paid for out of the
    /// padding rather than added to the box: `box-sizing: border-box` is on, so
    /// `px-3` plus a 1px border would be a 26px-wide gap between the border box
    /// and the text where the design asks for 24. It has no `sm:` variant — only
    /// the height and the type scale move at the breakpoint.
    #[must_use]
    pub fn padding_x(self) -> Pixels {
        let steps = match self {
            Self::Xl => 4.0,
            Self::Lg => 3.5,
            Self::Default => 3.0,
            Self::Sm => 2.5,
            Self::Xs => 2.0,
            Self::Icon | Self::IconSm | Self::IconLg | Self::IconXl | Self::IconXs => {
                return px(0.0);
            }
        };
        px(SPACING * steps - f32::from(BORDER_WIDTH))
    }

    /// `gap-2` from the base class list, which only `sm` (`gap-1.5`) and `xs`
    /// (`gap-1`) override. Every icon size keeps the base 8px, which is inert
    /// because an icon button has one child.
    #[must_use]
    pub fn gap(self) -> Pixels {
        px(SPACING
            * match self {
                Self::Sm => 1.5,
                Self::Xs => 1.0,
                _ => 2.0,
            })
    }

    /// `rounded-lg` from the base class list, which `xs` and `icon-xs` override
    /// with `rounded-md`.
    ///
    /// **Neither is Tailwind's stock value.** `theme.css` sets
    /// `--radius: 0.625rem` and `--radius-lg: var(--radius)`, so `rounded-lg` is
    /// **10px** rather than 8; `--radius-md: calc(var(--radius) * 0.8)` makes
    /// `rounded-md` **8px** rather than 6. Both confirmed on the live app —
    /// `borderTopLeftRadius: "10px"` on an `icon` button.
    #[must_use]
    pub fn radius(self, theme: &Theme) -> Pixels {
        match self {
            Self::Xs | Self::IconXs => theme.radius_md.value(),
            _ => theme.radius_lg.value(),
        }
    }

    /// `before:rounded-[calc(var(--radius-lg)-1px)]`, or the `--radius-md`
    /// spelling that `xs` and `icon-xs` swap in.
    ///
    /// The inner radius of a 1px border: the overlay sits on the host's
    /// **padding** box, one pixel inside the border box, so its corner is one
    /// pixel tighter. Measured live at **9px** on an `icon` button and **7px**
    /// on an `icon-xs` one, which is the arithmetic exactly.
    #[must_use]
    pub fn overlay_radius(self, theme: &Theme) -> Pixels {
        self.radius(theme) - BORDER_WIDTH
    }

    /// The type step, which is `text-base sm:text-sm` from the base class list
    /// on eight of the ten sizes.
    ///
    /// `xl` and `xs` each override both halves, and tailwind-merge keeps the
    /// later one **per variant group** — so `xl` is `text-lg sm:text-base` and
    /// `xs` is `text-sm sm:text-xs`, not a mixture. Read off the live app's
    /// class list, where both the base and the size's spelling are present and
    /// the merge has already happened.
    ///
    /// The token each step reads is the one carrying its number — the
    /// `--ui-text-*` trade `native/MAPPING.md` states once — except
    /// [`TEXT_LG`], which no token carries.
    #[must_use]
    pub fn type_step(self, theme: &Theme, breakpoint: Breakpoint) -> TypeStep {
        match (self, breakpoint) {
            // `text-lg` — the one step with no token.
            (Self::Xl, Breakpoint::Base) => TypeStep {
                size: TEXT_LG,
                line_height: LINE_HEIGHT_LG,
            },
            // `sm:text-base` = 1rem = `--ui-text-lg`, and the base list's own
            // `text-base` on the eight sizes that do not override it.
            (Self::Xl, Breakpoint::Sm) => TypeStep {
                size: theme.ui_text_lg.value(),
                line_height: LINE_HEIGHT_BASE,
            },
            // `text-xs` = 0.75rem = `--ui-text-sm`.
            (Self::Xs, Breakpoint::Sm) => TypeStep {
                size: theme.ui_text_sm.value(),
                line_height: LINE_HEIGHT_XS,
            },
            // `text-sm` = 0.875rem = `--ui-text-base`: `xs`'s unprefixed
            // override, and the base list's `sm:text-sm` everywhere else.
            (Self::Xs, Breakpoint::Base) | (_, Breakpoint::Sm) => TypeStep {
                size: theme.ui_text_base.value(),
                line_height: LINE_HEIGHT_SM,
            },
            // `text-base`, the base class list's unprefixed step.
            (_, Breakpoint::Base) => TypeStep {
                size: theme.ui_text_lg.value(),
                line_height: LINE_HEIGHT_BASE,
            },
        }
    }

    /// `[&_svg:not([class*='size-'])]:size-4.5 sm:…size-4`, and the two sizes
    /// that override it.
    ///
    /// `icon-xs`'s override is written
    /// `not-in-data-[slot=input-group]:[&_svg…]:size-4`, so it applies unless the
    /// button sits inside an `[data-slot=input-group]` ancestor. **A reading, not
    /// a measurement:** this port takes the not-in-a-group case, which is what
    /// the live app's two `icon-xs` buttons are — measured at a 12px glyph, which
    /// is the *call site's* own `size-3` beating this rule entirely.
    #[must_use]
    pub fn icon(self, breakpoint: Breakpoint) -> Pixels {
        let (base, small) = match self {
            Self::Xl | Self::IconXl => (5.0, 4.5),
            Self::IconXs => (4.0, 3.5),
            _ => (4.5, 4.0),
        };
        px(SPACING
            * match breakpoint {
                Breakpoint::Base => base,
                Breakpoint::Sm => small,
            })
    }

    /// The **in-flow** glyph's box: [`Size::icon`] with `[&_svg]:-mx-0.5`
    /// already folded into it.
    ///
    /// This is where the negative margin is resolved by hand rather than
    /// declared, because gpui cannot lay a negative margin out in a
    /// content-sized flex container. [`ICON_MARGIN_X`] carries the measurement
    /// and the reasoning; the short version is that the quantity CSS actually
    /// contributes to the button's width is the glyph's **margin box**, and this
    /// is that number.
    #[must_use]
    pub fn glyph_box(self, breakpoint: Breakpoint) -> Pixels {
        self.icon(breakpoint) + ICON_MARGIN_X * 2.0
    }
}

/// `cva`'s `variant` union, all seven of them.
///
/// **Six are live and one is dead**, counted over every `<Button` in
/// `web/src/`: `ghost` 56, `default` 53 (41 of them implicit), `outline` 18,
/// `destructive` 6, `secondary` 4, `link` 1, plus four written as a ternary
/// which resolve to `secondary`/`ghost`/`default`/`outline`. **`destructive-outline`
/// appears nowhere but its own definition.**
#[derive(Clone, Copy, Debug, Default, PartialEq, Eq)]
pub enum Variant {
    /// `border-primary bg-primary text-primary-foreground`. `cva`'s own default.
    #[default]
    Default,
    /// `border-destructive bg-destructive text-white`.
    Destructive,
    /// `border-input bg-popover text-destructive-foreground`. **Dead.**
    DestructiveOutline,
    /// `border-transparent text-foreground hover:bg-accent`. The most-used arm.
    Ghost,
    /// `border-transparent text-foreground hover:underline`.
    Link,
    /// `border-input bg-popover text-foreground`, `dark:bg-input/32`.
    Outline,
    /// `border-transparent bg-secondary text-secondary-foreground`.
    Secondary,
}

/// Every variant, in `cva`'s declaration order.
pub const ALL_VARIANTS: [Variant; 7] = [
    Variant::Default,
    Variant::Destructive,
    Variant::DestructiveOutline,
    Variant::Ghost,
    Variant::Link,
    Variant::Outline,
    Variant::Secondary,
];

/// `hover:bg-primary/90`, `data-pressed:bg-primary/90`, and the same pair on
/// `destructive` and `secondary`.
const TINT_90: f32 = 90.0;
/// `[:active,[data-pressed]]:bg-secondary/80` — see [`Variant::background`].
const TINT_80: f32 = 80.0;
/// `dark:hover:bg-input/64`, `dark:data-pressed:bg-input/64`.
const TINT_64: f32 = 64.0;
/// `hover:bg-accent/50`, `data-pressed:bg-accent/50`.
const TINT_50: f32 = 50.0;
/// `dark:bg-input/32` and `hover:border-destructive/32`.
const TINT_32: f32 = 32.0;
/// `hover:bg-destructive/4`.
const TINT_4: f32 = 4.0;

impl Variant {
    /// Its word on the command line and in a caption — `cva`'s own key.
    #[must_use]
    pub const fn name(self) -> &'static str {
        match self {
            Self::Default => "default",
            Self::Destructive => "destructive",
            Self::DestructiveOutline => "destructive-outline",
            Self::Ghost => "ghost",
            Self::Link => "link",
            Self::Outline => "outline",
            Self::Secondary => "secondary",
        }
    }

    /// Whether any live `<Button` in `web/src/` passes this variant.
    #[must_use]
    pub const fn live(self) -> bool {
        !matches!(self, Self::DestructiveOutline)
    }

    /// The border **colour**. The border's *width* is [`BORDER_WIDTH`] on every
    /// variant there is — see the module docs.
    ///
    /// `hover:border-destructive/32` and its `data-pressed:` twin are the only
    /// rules on this component that move a border colour, and they belong to the
    /// dead `destructive-outline` variant.
    #[must_use]
    pub fn border(self, theme: &Theme, state: ButtonState) -> Color {
        match self {
            Self::Default => theme.primary,
            Self::Destructive => theme.destructive,
            Self::DestructiveOutline if state.engaged() => {
                theme.destructive.mix(TINT_32, Color::TRANSPARENT)
            }
            Self::DestructiveOutline | Self::Outline => theme.input,
            // `border-transparent`, which is still a **one pixel** border.
            Self::Ghost | Self::Link | Self::Secondary => Color::TRANSPARENT,
        }
    }

    /// The background, or `None` for a variant that paints none.
    ///
    /// `None` rather than a transparent colour, because "paints nothing" and
    /// "paints `#00000000`" are the same picture and the first is the truer
    /// statement — and because gpui's `bg(…)` takes a `Fill`, so not calling it
    /// is how the absence is spelled.
    ///
    /// **The `active` prop is applied by the caller, not here.** See
    /// [`ACTIVE_TINT`]: it replaces the *resting* background only, and this
    /// function is where the resting/hover/pressed choice is made.
    #[must_use]
    pub fn background(self, theme: &Theme, state: ButtonState) -> Option<Color> {
        let dark = is_dark(theme);
        let engaged = state.engaged();
        match self {
            Self::Default if engaged => Some(theme.primary.mix(TINT_90, Color::TRANSPARENT)),
            Self::Default => Some(theme.primary),
            Self::Destructive if engaged => {
                Some(theme.destructive.mix(TINT_90, Color::TRANSPARENT))
            }
            Self::Destructive => Some(theme.destructive),
            // **`data-pressed` is 80%, not 90%.** `data-pressed:bg-secondary/90`
            // and `[:active,[data-pressed]]:bg-secondary/80` are the same
            // specificity — a class plus a one-simple-selector `:is()` — so
            // source order decides, and Tailwind emits the arbitrary-variant
            // rule **later**. Hover alone is still 90%.
            Self::Secondary if state.interaction.pressed => {
                Some(theme.secondary.mix(TINT_80, Color::TRANSPARENT))
            }
            Self::Secondary if state.interaction.hovered => {
                Some(theme.secondary.mix(TINT_90, Color::TRANSPARENT))
            }
            Self::Secondary => Some(theme.secondary),
            Self::DestructiveOutline if engaged => {
                Some(theme.destructive.mix(TINT_4, Color::TRANSPARENT))
            }
            Self::Outline if engaged && dark => Some(theme.input.mix(TINT_64, Color::TRANSPARENT)),
            Self::Outline if engaged => Some(theme.accent.mix(TINT_50, Color::TRANSPARENT)),
            // `bg-popover`, with `dark:bg-input/32` layered over it. The `dark:`
            // variant is the same switch that selects the token table, which is
            // why `is_dark` can answer it without an `Appearance` argument.
            Self::DestructiveOutline | Self::Outline if dark => {
                Some(theme.input.mix(TINT_32, Color::TRANSPARENT))
            }
            Self::DestructiveOutline | Self::Outline => Some(theme.popover),
            Self::Ghost if engaged => Some(theme.accent),
            // `link` paints no background in any state: its only interaction
            // rule is `hover:underline`.
            Self::Ghost | Self::Link => None,
        }
    }

    /// The text colour.
    ///
    /// **`destructive` is the awkward one.** `text-white` compiles to
    /// `color: var(--color-white)`, which is Tailwind's own token and not one
    /// `Theme` carries a field for. The port reads `Theme::LIGHT.card`, which is
    /// not a coincidence of value but the same declaration: `theme.css` writes
    /// `--card: var(--color-white)` in `:root`, so the light table's `card`
    /// **is** `--color-white`, `#ffffffff`. This is `native/MAPPING.md`'s
    /// `--ui-text-*` trade in a second place — read the token that carries the
    /// value and say so at the call site — and it is what keeps
    /// `scripts/check-invariants.sh` rule 4 unbroken without minting a colour
    /// the design system does not have.
    ///
    /// It is deliberately **not** `theme.card`: that is the *current* table's,
    /// which in dark is the background, and the CSS says white in both.
    #[must_use]
    pub fn foreground(self, theme: &Theme) -> Color {
        match self {
            Self::Default => theme.primary_foreground,
            Self::Destructive => Theme::LIGHT.card,
            Self::DestructiveOutline => theme.destructive_foreground,
            Self::Ghost | Self::Link | Self::Outline => theme.foreground,
            Self::Secondary => theme.secondary_foreground,
        }
    }

    /// The colour `*:data-[slot=button-loading-indicator]:text-*` gives the
    /// spinner, which is the variant's own foreground except on the two
    /// `outline` arms.
    ///
    /// **Invisible to the differ**: the spinner is an `<svg>`, the port renders
    /// it as an empty box (the same call every component since `git_status_row`
    /// has made about icons), and a box with no text emits no `fg`. Recorded
    /// because the rule exists and a reader will look for it.
    #[must_use]
    pub fn indicator_foreground(self, theme: &Theme) -> Color {
        match self {
            Self::Default => theme.primary_foreground,
            Self::Destructive => Theme::LIGHT.card,
            Self::Secondary => theme.secondary_foreground,
            Self::DestructiveOutline | Self::Ghost | Self::Link | Self::Outline => theme.foreground,
        }
    }

    /// `hover:underline` / `data-pressed:underline` — `link`'s only interaction
    /// rule, and the only text decoration on the component.
    #[must_use]
    pub fn underlined(self, state: ButtonState) -> bool {
        matches!(self, Self::Link) && state.engaged()
    }
}

/// Whether a `dark:` Tailwind variant is in force.
///
/// A local copy of the one `git_status_row` and `dropdown_menu` each carry, kept
/// local deliberately: the two components are ported independently and a shared
/// helper would make one surface's diff reach into the other's file. The React
/// app switches on a `dark` class on the document element, which is the same
/// switch that selects the token table.
fn is_dark(theme: &Theme) -> bool {
    *theme == Theme::DARK
}

/// The three CSS pseudo-classes a pointer or the keyboard puts a button in.
///
/// **Parameters, not gpui `.hover(…)` refinements** — `ANCHORS.md` §6 makes
/// runtime interaction state invisible to the extractor, so a component that
/// expressed its states that way would report its resting paint in every cell.
#[derive(Clone, Copy, Debug, Default, PartialEq, Eq)]
pub struct Interaction {
    /// `:hover`. **The first genuinely visible interaction state in this port**:
    /// `hover:bg-*` changes `bg` on six of the seven variants, which is a
    /// compared field — unlike `dropdown-menu`'s focus (also a background, but
    /// on a surface whose reference cannot be driven either) and unlike
    /// `resizable`'s, which paints a pseudo-element §3 forbids anchoring.
    ///
    /// It still has **no reference**: `CGPreflightPostEventAccess()` is false on
    /// this project's machines, so no capture can be taken with a pointer over a
    /// button. The rule compiles to `@media (hover: hover)`, which the live app
    /// reports as **true**, so the state is reachable by a human and not by this
    /// item.
    pub hovered: bool,
    /// `data-pressed` (base-ui writes it while the pointer is down) and the CSS
    /// `:active` that every `[:active,[data-pressed]]:` rule pairs it with. One
    /// field, because every rule on this component names both.
    pub pressed: bool,
    /// `:focus-visible`. A real state — the button is focusable — whose only
    /// rules are `ring-2`, `ring-offset-1` and `outline-hidden`: two box-shadows
    /// and an outline, and `ANCHORS.md` §6 has a field for neither.
    pub focused: bool,
}

/// The three optional props of `button.tsx` that change what it paints.
///
/// Split from [`Interaction`] rather than flattened beside it, and the split is
/// the surface's own: a pseudo-class is a §8.3 **flag** on the command line and
/// a prop is a surface **option**. (It also keeps either struct inside clippy's
/// `struct_excessive_bools`, which is what forced the question — but a flat
/// six-field bag would have hidden a real division.)
#[derive(Clone, Copy, Debug, Default, PartialEq, Eq)]
pub struct Props {
    /// `disabled`. **Live** — 35 of the 142 `<Button` call sites pass it — and
    /// **invisible**: its three rules are `pointer-events-none` (not a visual
    /// property), `opacity-64` (no field, and non-zero so v1.7's `visible` term
    /// does not fire) and `shadow-none` (§6).
    pub disabled: bool,
    /// `loading`. Sets `data-loading`, which makes the label **transparent** —
    /// the one thing on this component that changes `fg` — and renders the
    /// spinner. It also forces `disabled`, because `button.tsx` computes
    /// `isDisabled = Boolean(loading || disabledProp)`; ask
    /// [`ButtonState::is_disabled`] rather than this field.
    ///
    /// **No live call site passes it.**
    pub loading: bool,
    /// `active` — `bg-accent/20` over the resting background. **No live call
    /// site passes it**, exactly as no live consumer passes `SidebarTreeRow`'s
    /// `active`.
    pub active: bool,
}

/// The visual state a button is painted in: its pseudo-classes and its props.
#[derive(Clone, Copy, Debug, Default, PartialEq, Eq)]
pub struct ButtonState {
    /// The CSS pseudo-classes.
    pub interaction: Interaction,
    /// The props.
    pub props: Props,
}

impl ButtonState {
    /// The resting button.
    #[must_use]
    pub const fn resting() -> Self {
        Self {
            interaction: Interaction {
                hovered: false,
                pressed: false,
                focused: false,
            },
            props: Props {
                disabled: false,
                loading: false,
                active: false,
            },
        }
    }

    /// Whether the pointer-driven rules are in force.
    ///
    /// Every variant's `hover:bg-*` and `data-pressed:bg-*` name the **same**
    /// colour, with one exception (`secondary`, see [`Variant::background`]), so
    /// asking the two questions separately at every call site would be six
    /// duplicated branches and one real one.
    #[must_use]
    pub const fn engaged(self) -> bool {
        self.interaction.hovered || self.interaction.pressed
    }

    /// `isDisabled = Boolean(loading || disabledProp)`, from `button.tsx`.
    #[must_use]
    pub const fn is_disabled(self) -> bool {
        self.props.disabled || self.props.loading
    }
}

/// Which of the three §8.3 content lengths a labelled button shows.
///
/// A type of its own rather than `ContentLength` reused: the git row's fixture
/// strings are **paths**, chosen so the middle one truncates against a
/// directory, and a button label is a verb phrase. Sharing the strings would put
/// `src/features/.../resolve-terminal-connection.ts` on a button, which is not a
/// picture the app can produce.
#[derive(Clone, Copy, Debug, Default, PartialEq, Eq)]
pub enum Label {
    /// A one-word label.
    Short,
    /// A typical two-word label.
    #[default]
    Normal,
    /// The longest label the app has room for.
    ///
    /// It cannot **clip**: the base class list has `whitespace-nowrap` and no
    /// `overflow-hidden`, and `shrink-0` stops the flex line squeezing it, so a
    /// long label makes the button wider rather than truncating it. That also
    /// disposes of `dropdown-menu`'s wrapping trap — see [`Button::render`].
    Overflow,
}

impl Label {
    /// The string this length shows.
    #[must_use]
    pub const fn text(self) -> &'static str {
        match self {
            Self::Short => "Add",
            Self::Normal => "Create workspace",
            Self::Overflow => "Import an existing repository",
        }
    }
}

/// A `rounded-*` class a **call site** merges over the size variant's own.
///
/// # Why this is a parameter, and where the line is
///
/// `button.tsx` composes `cn(buttonVariants({ className, size, variant }), …)`,
/// so a call site's `className` is merged over the variant's by tailwind-merge —
/// **the call site is half the component**. Every one of the nine live Buttons
/// does it, and this is the one merged class that reaches a *compared* field.
///
/// It names the **class**, never the number, and resolves through the sealed
/// token the class names. That is the whole difference between an input both
/// engines still have to resolve and an answer handed to the port: a
/// `--radius 6` would let a cell be tuned to whatever the reference happened to
/// say, and the anchor would stop being able to fail. Same shape as
/// [`Props::loading`] — a fact about the call site, not about the layout.
///
/// The three arms are the three classes live call sites actually write:
/// `rounded-sm` at five of the nine, `rounded-lg` at two, and none at the other
/// two. There is deliberately no `rounded-xl` or `rounded-none`.
#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub enum RadiusClass {
    /// `rounded-sm` — `calc(var(--radius) * 0.6)` = **6px**.
    ///
    /// What the tab bar's five buttons write, verbatim, in
    /// `tab-navigation-buttons.tsx`, `tab-add-button.tsx`,
    /// `close-split-button.tsx` and `sidebar-project-header.tsx` (twice). The
    /// first of those carries a comment naming the intent — "icon-sm (28px, 6px
    /// radius)" — so this is the design's number and not a stray override.
    Sm,
    /// `rounded-md` — `calc(var(--radius) * 0.8)` = **8px**. No live call site
    /// writes it as an override; it is the `xs`/`icon-xs` *variants'* own class,
    /// and it is here so the vocabulary is the token scale rather than a subset
    /// chosen by what happened to be on screen.
    Md,
    /// `rounded-lg` — `var(--radius)` = **10px**, not Tailwind's stock 8.
    ///
    /// What the two `icon-xs` buttons in the workspace header write, *over* the
    /// variant's own `rounded-md`.
    Lg,
}

/// Every `rounded-*` class the surface can be driven to, small to large.
pub const ALL_RADIUS_CLASSES: [RadiusClass; 3] =
    [RadiusClass::Sm, RadiusClass::Md, RadiusClass::Lg];

impl RadiusClass {
    /// Its word on the command line and in a caption — the Tailwind suffix.
    #[must_use]
    pub const fn name(self) -> &'static str {
        match self {
            Self::Sm => "sm",
            Self::Md => "md",
            Self::Lg => "lg",
        }
    }

    /// The class's compiled value, read out of the sealed token it names.
    ///
    /// **Through the token, never as a literal.** `theme.css` defines the whole
    /// scale off one `--radius: 0.625rem` — `--radius-sm: calc(var(--radius) *
    /// 0.6)`, `--radius-md: calc(… * 0.8)`, `--radius-lg: var(--radius)` — so a
    /// project that moved the base moves all three together, and a port holding
    /// `px(6.0)` would silently stop describing the app.
    #[must_use]
    pub fn value(self, theme: &Theme) -> Pixels {
        match self {
            Self::Sm => theme.radius_sm.value(),
            Self::Md => theme.radius_md.value(),
            Self::Lg => theme.radius_lg.value(),
        }
    }
}

/// One `<Button>`.
#[derive(Clone, Debug, PartialEq)]
pub struct Button {
    /// The anchor the `<button>` carries.
    ///
    /// Authored rather than fixed, because `button.tsx` writes [`ID_BUTTON`]
    /// *before* `{...props}` and there are nine Buttons in one live document —
    /// so a capture of any but the first has to name it at the call site or
    /// scope to it.
    pub id: SharedString,
    /// `variant`.
    pub variant: Variant,
    /// `size`.
    pub size: Size,
    /// Which side of the `sm` breakpoint the **viewport** is on.
    ///
    /// A parameter for the reason `git_status_row`'s is: a media query asks
    /// about the window, not about the box this component is drawn in, and gpui
    /// has no media queries at all.
    pub breakpoint: Breakpoint,
    /// Whether a leading icon is rendered.
    ///
    /// An **empty box**, as every icon in this port is: the glyph is an SVG a
    /// call site chooses, there is no native equivalent, and drawing a
    /// substitute would put a shape on screen for the oracle to converge on.
    /// On by default in the fixture, because all nine live Buttons are
    /// icon-only.
    pub icon: bool,
    /// The label, or `None` for the icon-only shape every live call site has.
    pub label: Option<Label>,
    /// A call site's own `rounded-*`, or `None` for the size variant's.
    ///
    /// See [`RadiusClass`] for why a call-site class is a parameter at all.
    /// `None` is the *primitive's* radius — and it has **no visible reference**:
    /// the only two live Buttons that keep it sit inside the sidebar carousel's
    /// snapped-out panels and report `visible: false`, so no capture of the
    /// unmerged button is comparable. `Button::fixture` therefore defaults to
    /// `Some(RadiusClass::Sm)`, which is the button a reference *can* be taken
    /// from.
    pub class_radius: Option<RadiusClass>,
    /// The visual state.
    pub state: ButtonState,
}

impl Button {
    /// The button a reference can actually be taken from: `variant="ghost"
    /// size="icon-sm"` with the tab bar's `rounded-sm`, one icon, no label, at
    /// rest.
    ///
    /// Read off the live app rather than chosen. The nine `[data-slot=button]`
    /// elements in the fixture workspace divide into three groups, and only one
    /// of them is capturable:
    ///
    /// | group | radius | `visible` |
    /// |---|---|---|
    /// | five in the tab bar, `rounded-sm` over the variant | **6** | true |
    /// | two `icon-xs` in the workspace header, `rounded-lg` over `rounded-md` | 10 | true |
    /// | two with **no** `rounded-*` override — the primitive's own | 10 | **false** |
    ///
    /// The two unmerged ones are inside the sidebar carousel's snapped-out
    /// panels, so their whole snapshot is an artefact of an ancestor this port
    /// has no equivalent for (`ANCHORS.md` v1.7's own worked example). **There
    /// is no live Button that is both unmerged and visible**, so the fixture is
    /// the merged one and [`Button::class_radius`] is what says so.
    ///
    /// The call site is `web/src/features/tabs/components/tab-navigation-buttons.tsx`
    /// — the sidebar toggle, `aria-label="Hide sidebar"`, measured at
    /// `x 88, y 8, 28×28, radius 6`. Four more Buttons write the same class
    /// string verbatim.
    ///
    /// **`size-8 sm:size-7` is `icon-sm`, not `icon`.** `icon` is
    /// `size-9 sm:size-8`, one step larger, and reading the reference's class
    /// list as the wrong arm is a four-pixel error in both axes — caught here by
    /// `the_fixture_is_the_reference_button`, which asserts the 28 the live
    /// element measures.
    #[must_use]
    pub fn fixture() -> Self {
        Self {
            id: SharedString::new_static(ID_BUTTON),
            variant: Variant::Ghost,
            size: Size::IconSm,
            breakpoint: Breakpoint::Sm,
            icon: true,
            label: None,
            class_radius: Some(RadiusClass::Sm),
            state: ButtonState::resting(),
        }
    }

    /// The radius the button is drawn with: a call site's class where there is
    /// one, and the size variant's own where there is not.
    ///
    /// **The `::before` overlay does not follow it**, and that is measured
    /// rather than reasoned: `before:rounded-[…]` is a *different* tailwind-merge
    /// group from `rounded-*`, so a call site's `rounded-sm` moves the host and
    /// leaves the overlay on the variant's class. Live, on the fixture's own
    /// button: host **6px**, `::before` **9px** — and the reciprocal on the two
    /// `icon-xs` buttons, whose call-site `rounded-lg` gives host **10px** with
    /// `::before` at the variant's **7px**. See [`Button::overlay`].
    #[must_use]
    pub fn radius(&self, theme: &Theme) -> Pixels {
        self.class_radius
            .map_or_else(|| self.size.radius(theme), |class| class.value(theme))
    }

    /// Renders the button, opting every contract anchor into `anchors`.
    ///
    /// The children go in `button.tsx`'s own order — `{children}` then the
    /// spinner — which is why the label is placed through
    /// [`AnchorSink::text_half`] rather than through
    /// [`AnchorSink::boxed_text`]: the default appends the run **last**, which
    /// would put the spinner before the label. The same call `dropdown_menu`
    /// makes about its chevron.
    ///
    /// `whitespace-nowrap` is the rare class that ports to something and matters:
    /// gpui only computes a wrap width when `white_space` is `Normal`
    /// (`elements/text.rs`), so `.whitespace_nowrap()` is what makes a long label
    /// shape on one line here as it does in `WebKit`. Without it a label wider
    /// than the surface would wrap on the native side, and a wrapped run is
    /// outside what the contract can compare at all — `native/MAPPING.md` records
    /// that as a `dropdown-menu` trap, and this component is immune to it by
    /// declaration rather than by luck.
    #[must_use]
    pub fn render(&self, theme: &Theme, anchors: &dyn AnchorSink) -> AnyElement {
        let id = AnchorId::new(self.id.clone());
        let mut element = self.shell(theme).child(self.overlay(theme));

        if self.icon {
            element = element.child(self.glyph());
        }
        if let Some(label) = self.label {
            element = element.child(anchors.text_half(&id, label.text().into()));
        }
        if self.state.props.loading {
            element =
                element.child(anchors.boxed(ID_LOADING_INDICATOR.into(), self.spinner(theme)));
        }

        anchors.root(id, element).into_any_element()
    }

    /// The `<button>`'s own box.
    ///
    /// `inline-flex` becomes `.flex()`, and that is not a loss: gpui has no
    /// inline flow at all, and the reference's own computed `display` is
    /// **`flex`** rather than `inline-flex` — measured live, because CSS
    /// blockifies the display of a flex item and every live Button is one. The
    /// surface reproduces that by making its host a flex row, which is also what
    /// gives a labelled button its shrink-to-fit width.
    fn shell(&self, theme: &Theme) -> Div {
        let extent = self.size.extent(self.breakpoint);
        let step = self.size.type_step(theme, self.breakpoint);

        let mut element = div()
            .relative()
            .flex()
            .flex_shrink_0()
            .items_center()
            .justify_center()
            .gap(self.size.gap())
            .whitespace_nowrap()
            .h(extent)
            .rounded(self.radius(theme))
            .border_1()
            .border_color(self.variant.border(theme, self.state))
            .text_size(step.size)
            .line_height(relative(step.line_height))
            .font_weight(FontWeight::MEDIUM)
            .text_color(self.foreground(theme));

        if self.size.square() {
            element = element.w(extent);
        } else {
            element = element.px(self.size.padding_x());
        }
        if let Some(background) = self.background(theme) {
            element = element.bg(background);
        }
        if self.variant.underlined(self.state) {
            element = element.underline();
        }
        if self.state.is_disabled() {
            element = element.opacity(DISABLED_OPACITY);
        }
        if self.state.interaction.focused {
            element = ring(element, theme);
        }
        element
    }

    /// The background the button ends up with, once the `active` prop has had
    /// its say.
    ///
    /// [`ACTIVE_TINT`] replaces the **resting** background and loses to
    /// `hover:`/`data-pressed:`, which is a fact about CSS specificity rather
    /// than about tailwind-merge — see the constant.
    fn background(&self, theme: &Theme) -> Option<Color> {
        if self.state.props.active && !self.state.engaged() {
            return Some(theme.accent.mix(ACTIVE_TINT, Color::TRANSPARENT));
        }
        self.variant.background(theme, self.state)
    }

    /// The text colour, after `data-loading:text-transparent`.
    ///
    /// **The only field on this component that a state flag moves and the
    /// contract can see.** Everything else `loading` does is either a shadow, an
    /// opacity or a `user-select`.
    fn foreground(&self, theme: &Theme) -> Color {
        if self.state.props.loading {
            Color::TRANSPARENT
        } else {
            self.variant.foreground(theme)
        }
    }

    /// The `::before` overlay: `absolute inset-0` with the inner radius.
    ///
    /// **Deliberately unanchored, and for a reason particular to this component.**
    /// `ANCHORS.md` §3's pseudo-backed shortcut *is* valid here — the pseudo is
    /// `position: absolute; inset: 0`, which is exactly the case the rule
    /// permits — but taking it would **replace** the host's own record:
    /// `extractSnapshot` reads `pseudoMap[id]` off the element carrying the
    /// `data-oracle-id`, and a pseudo-backed anchor reports the pseudo's paint
    /// over the host's padding box. The host here is the button, whose
    /// background, border and 10px radius are the whole point of the surface. So
    /// the overlay is painted and not anchored, and its 9px inner radius lives in
    /// [`Size::overlay_radius`] where it can at least be asserted.
    ///
    /// **Its radius is the size variant's, never a call site's.**
    /// `before:rounded-[calc(var(--radius-lg)-1px)]` and a bare `rounded-sm` are
    /// different tailwind-merge groups, so both survive the merge and each
    /// applies to its own box. Measured on all nine live Buttons: the five with
    /// `rounded-sm` are `6 / 9`, the two with `rounded-lg` over `icon-xs` are
    /// `10 / 7`, and the two unmerged are `10 / 9`. A port that let
    /// [`Button::class_radius`] reach here would be wrong on seven of the nine —
    /// invisibly, because the overlay is unanchored.
    ///
    /// Its four shadow layers are **not painted**. Each one is
    /// `--theme(--color-black/N%)` or `--theme(--color-white/N%)`, and
    /// `scripts/check-invariants.sh` rule 4 will not let a component outside
    /// `crowbar-ui/src/theme/` mint either — nor should it, because no crowbar
    /// token is `--color-black` at 4% the way `--card` *is* `--color-white`.
    /// Nothing is lost that the differ could see: `ANCHORS.md` §6 has no field
    /// for a shadow.
    fn overlay(&self, theme: &Theme) -> Div {
        div()
            .absolute()
            .top(px(0.0))
            .right(px(0.0))
            .bottom(px(0.0))
            .left(px(0.0))
            .rounded(self.size.overlay_radius(theme))
    }

    /// A call site's leading `<svg>`, as an empty box.
    ///
    /// Three of the base class list's four svg rules land here: `size-4.5`/
    /// `sm:size-4` (or a size's own override), `shrink-0`, and `-mx-0.5` —
    /// the last **resolved into the box** rather than declared, because gpui
    /// cannot lay a negative margin out in a content-sized flex container. See
    /// [`ICON_MARGIN_X`] for the measurement and [`Size::glyph_box`] for the
    /// arithmetic. `pointer-events-none` is not a visual property.
    ///
    /// The height keeps the true [`Size::icon`]: `-mx-0.5` is an *inline*
    /// margin and the cross axis is untouched.
    fn glyph(&self) -> Div {
        div()
            .flex_shrink_0()
            .w(self.size.glyph_box(self.breakpoint))
            .h(self.size.icon(self.breakpoint))
    }

    /// `<Spinner className="pointer-events-none absolute" />`.
    ///
    /// An `<svg>` like any other, so an empty box of the same size — and
    /// `absolute` with no inset, which takes its static position from the flex
    /// container's own alignment. Both engines centre it: `items-center` and
    /// `justify-center` are on the button, CSS Flexbox §4.1 says an
    /// absolutely-positioned child is placed as though it were the sole flex
    /// item, and taffy applies the container's alignment to an auto inset the
    /// same way — the measurement `dropdown-menu` already made for its `right-2`
    /// tick.
    ///
    /// It carries `-mx-0.5` too: `[&_svg]:-mx-0.5` is a descendant selector and
    /// the spinner is a descendant. Inert on an out-of-flow box, and reproduced
    /// rather than tidied.
    fn spinner(&self, theme: &Theme) -> Div {
        div()
            .absolute()
            .flex_shrink_0()
            .w(self.size.icon(self.breakpoint))
            .h(self.size.icon(self.breakpoint))
            .mx(ICON_MARGIN_X)
            .text_color(self.variant.indicator_foreground(theme))
    }
}

/// Adds `focus-visible:ring-2 ring-offset-1`'s two shadow layers in front of
/// whatever shadows `element` already carries.
///
/// A local copy of the one `dropdown_menu` and `resizable` each carry, for their
/// reason: the components are ported independently and a shared helper would make
/// one surface's diff reach into another's file. What is new here is the second
/// layer — `resizable`'s `ring-offset-width` stays at its `0px` initial and paints
/// nothing, while `ring-offset-1` moves this one to 1px. Tailwind's composite puts
/// the offset **in front of** the ring, so the two are inserted in that order.
fn ring(mut element: Div, theme: &Theme) -> Div {
    let offset =
        BoxShadow::new(px(0.0), px(0.0), theme.background.value()).spread_radius(RING_OFFSET_WIDTH);
    let halo = BoxShadow::new(px(0.0), px(0.0), theme.ring.value()).spread_radius(RING_SPREAD);
    let shadows = element.style().box_shadow.get_or_insert_with(Vec::new);
    shadows.insert(0, halo);
    shadows.insert(0, offset);
    element
}

#[cfg(test)]
mod tests {
    use super::{
        ACTIVE_TINT, ALL_RADIUS_CLASSES, ALL_SIZES, ALL_VARIANTS, BORDER_WIDTH, Breakpoint, Button,
        ButtonState, CONTENT_SIZED, DISABLED_OPACITY, ICON_MARGIN_X, ID_BUTTON,
        ID_LOADING_INDICATOR, Interaction, LINE_SIZED, Label, Props, RING_OFFSET_WIDTH,
        RING_SPREAD, RadiusClass, Size, TEXT_LG, Variant,
    };
    use crate::theme::{Color, Theme};
    use gpui::{Rems, px};

    /// A hovered button, spelled once because six tests need it.
    fn hovering() -> ButtonState {
        ButtonState {
            interaction: Interaction {
                hovered: true,
                ..Interaction::default()
            },
            ..ButtonState::resting()
        }
    }

    /// A pressed button.
    fn pressing() -> ButtonState {
        ButtonState {
            interaction: Interaction {
                pressed: true,
                ..Interaction::default()
            },
            ..ButtonState::resting()
        }
    }

    /// **The border is one pixel wide in every variant**, which is the one thing
    /// about this component a port is most likely to get wrong — and the exact
    /// inverse of the `ring-1` trap `native/MAPPING.md` records twice.
    ///
    /// `border.w` is the field `ANCHORS.md` v1.1 compares *exactly*, so a ghost
    /// button rendered without it is a delta on every cell of the matrix.
    #[test]
    fn every_variant_has_a_one_pixel_border_and_three_of_them_paint_it_transparent() {
        assert_eq!(BORDER_WIDTH, px(1.0));

        for theme in [Theme::LIGHT, Theme::DARK] {
            let transparent: Vec<&str> = ALL_VARIANTS
                .into_iter()
                .filter(|variant| {
                    variant.border(&theme, ButtonState::resting()) == Color::TRANSPARENT
                })
                .map(Variant::name)
                .collect();
            assert_eq!(transparent, ["ghost", "link", "secondary"]);

            // And the other four name a token, none of which is transparent.
            for variant in [
                Variant::Default,
                Variant::Destructive,
                Variant::DestructiveOutline,
                Variant::Outline,
            ] {
                assert_ne!(
                    variant.border(&theme, ButtonState::resting()),
                    Color::TRANSPARENT,
                    "{}",
                    variant.name(),
                );
            }
        }
    }

    /// Every authored length, against the `calc(var(--spacing) * n)` the app's
    /// own Tailwind 4.3.0 compiles the class to — as the arithmetic rather than
    /// as more literals, so a change to the step cannot pass by leaving the
    /// literals alone.
    #[test]
    fn every_extent_is_the_compiled_spacing_multiple() {
        const STEP: f32 = 4.0;

        for (size, base, small) in [
            (Size::Xl, 11.0, 10.0),
            (Size::Lg, 10.0, 9.0),
            (Size::Default, 9.0, 8.0),
            (Size::Sm, 8.0, 7.0),
            (Size::Xs, 7.0, 6.0),
        ] {
            assert_eq!(size.extent(Breakpoint::Base), px(STEP * base), "{size:?}");
            assert_eq!(size.extent(Breakpoint::Sm), px(STEP * small), "{size:?}");
        }

        // Every text size shares its extent with its icon twin — `h-9` and
        // `size-9` are the same multiple — so this is five numbers, not ten.
        for (text, icon) in [
            (Size::Xl, Size::IconXl),
            (Size::Lg, Size::IconLg),
            (Size::Default, Size::Icon),
            (Size::Sm, Size::IconSm),
            (Size::Xs, Size::IconXs),
        ] {
            for breakpoint in [Breakpoint::Base, Breakpoint::Sm] {
                assert_eq!(text.extent(breakpoint), icon.extent(breakpoint), "{text:?}");
            }
        }
    }

    /// **The `sm:` variant is always exactly one `--spacing` step smaller**,
    /// which is the whole of what the viewport axis does to this component's
    /// box — and the reason `--viewport-width` is its strongest axis.
    #[test]
    fn the_sm_breakpoint_takes_exactly_one_step_off_every_size() {
        for size in ALL_SIZES {
            let base = size.extent(Breakpoint::Base);
            let small = size.extent(Breakpoint::Sm);
            assert_eq!(base - small, px(4.0), "{}", size.name());
        }
    }

    /// `px-[calc(--spacing(n)-1px)]` pays for the border out of the padding, and
    /// an icon size has none at all.
    #[test]
    fn the_horizontal_padding_is_a_spacing_step_less_the_border() {
        for (size, steps) in [
            (Size::Xl, 4.0),
            (Size::Lg, 3.5),
            (Size::Default, 3.0),
            (Size::Sm, 2.5),
            (Size::Xs, 2.0),
        ] {
            assert_eq!(size.padding_x(), px(4.0 * steps - 1.0), "{}", size.name());
            assert!(!size.square(), "{}", size.name());
        }

        for size in ALL_SIZES.into_iter().filter(|size| size.square()) {
            assert_eq!(size.padding_x(), px(0.0), "{}", size.name());
        }
        assert_eq!(ALL_SIZES.into_iter().filter(|s| s.square()).count(), 5);
    }

    /// **Neither radius is Tailwind's stock value.** `theme.css` redefines
    /// `--radius-lg` over a `0.625rem` base, and the overlay's is one pixel
    /// tighter because it sits on the padding box.
    #[test]
    fn the_radii_are_this_projects_and_the_overlay_is_a_pixel_inside_them() {
        for theme in [Theme::LIGHT, Theme::DARK] {
            assert_eq!(theme.radius_lg.value(), px(10.0));
            assert_eq!(theme.radius_md.value(), px(8.0));

            for size in ALL_SIZES {
                let expected = if matches!(size, Size::Xs | Size::IconXs) {
                    px(8.0)
                } else {
                    px(10.0)
                };
                assert_eq!(size.radius(&theme), expected, "{}", size.name());
                assert_eq!(
                    size.overlay_radius(&theme),
                    expected - BORDER_WIDTH,
                    "{}",
                    size.name(),
                );
            }
        }
        // The two the live app was measured at: 10/9 on an `icon` button and
        // 8/7 on an `icon-xs` one.
        assert_eq!(Size::Icon.overlay_radius(&Theme::DARK), px(9.0));
        assert_eq!(Size::IconXs.overlay_radius(&Theme::DARK), px(7.0));
    }

    /// The `--ui-text-*` trade, and the one step where it runs out.
    ///
    /// Three of Tailwind's four steps have a crowbar token carrying the same
    /// number; `text-lg` does not, and reading `--ui-text-xl` for it would paint
    /// 20px where the reference paints 18.
    #[test]
    fn the_type_scale_reads_the_token_that_carries_the_number_except_text_lg() {
        for theme in [Theme::LIGHT, Theme::DARK] {
            // `text-base` = 1rem = `--ui-text-lg`.
            assert_eq!(theme.ui_text_lg.value(), Rems(1.0));
            // `text-sm` = 0.875rem = `--ui-text-base`.
            assert_eq!(theme.ui_text_base.value(), Rems(0.875));
            // `text-xs` = 0.75rem = `--ui-text-sm`.
            assert_eq!(theme.ui_text_sm.value(), Rems(0.75));
            // `text-lg` = 1.125rem, and `--ui-text-xl` is 1.25 — a different
            // number, which is why `TEXT_LG` is a literal.
            assert_eq!(TEXT_LG, Rems(1.125));
            assert_ne!(theme.ui_text_xl.value(), TEXT_LG);

            // Eight of the ten sizes take the base list's own pair.
            for size in ALL_SIZES
                .into_iter()
                .filter(|s| !matches!(s, Size::Xl | Size::Xs))
            {
                assert_eq!(
                    size.type_step(&theme, Breakpoint::Base).size,
                    theme.ui_text_lg.value(),
                    "{}",
                    size.name(),
                );
                assert_eq!(
                    size.type_step(&theme, Breakpoint::Sm).size,
                    theme.ui_text_base.value(),
                    "{}",
                    size.name(),
                );
            }

            // And the two that override both halves override them together.
            assert_eq!(Size::Xl.type_step(&theme, Breakpoint::Base).size, TEXT_LG);
            assert_eq!(
                Size::Xl.type_step(&theme, Breakpoint::Sm).size,
                theme.ui_text_lg.value(),
            );
            assert_eq!(
                Size::Xs.type_step(&theme, Breakpoint::Base).size,
                theme.ui_text_base.value(),
            );
            assert_eq!(
                Size::Xs.type_step(&theme, Breakpoint::Sm).size,
                theme.ui_text_sm.value(),
            );
        }
    }

    /// Tailwind's line heights are the stock unitless pairs, and every one of
    /// them resolves to a whole number of pixels at the size it goes with.
    #[test]
    fn the_line_heights_are_tailwinds_stock_pairs() {
        let theme = Theme::DARK;
        for (size, breakpoint, px_size, px_line) in [
            (Size::Default, Breakpoint::Base, 16.0, 24.0),
            (Size::Default, Breakpoint::Sm, 14.0, 20.0),
            (Size::Xl, Breakpoint::Base, 18.0, 28.0),
            (Size::Xl, Breakpoint::Sm, 16.0, 24.0),
            (Size::Xs, Breakpoint::Base, 14.0, 20.0),
            (Size::Xs, Breakpoint::Sm, 12.0, 16.0),
        ] {
            let step = size.type_step(&theme, breakpoint);
            // gpui's rem is 16px, exactly as the document's root font size is.
            assert!((step.size.0 * 16.0 - px_size).abs() < 0.001, "{size:?}");
            assert!(
                (step.size.0 * 16.0 * step.line_height - px_line).abs() < 0.01,
                "{size:?} {breakpoint:?}",
            );
        }
    }

    /// **`data-pressed` on `secondary` is 80%, not 90%** — the one place two
    /// rules of equal specificity disagree and source order decides.
    #[test]
    fn a_pressed_secondary_button_is_eighty_percent_and_a_hovered_one_is_ninety() {
        for theme in [Theme::LIGHT, Theme::DARK] {
            let hovered = hovering();
            let pressed = pressing();

            assert_eq!(
                Variant::Secondary.background(&theme, hovered),
                Some(theme.secondary.mix(90.0, Color::TRANSPARENT)),
            );
            assert_eq!(
                Variant::Secondary.background(&theme, pressed),
                Some(theme.secondary.mix(80.0, Color::TRANSPARENT)),
            );
            assert_ne!(
                Variant::Secondary.background(&theme, hovered),
                Variant::Secondary.background(&theme, pressed),
            );

            // Every other variant names the same colour for both, which is why
            // `engaged()` is one question rather than two.
            for variant in ALL_VARIANTS
                .into_iter()
                .filter(|v| !matches!(v, Variant::Secondary))
            {
                assert_eq!(
                    variant.background(&theme, hovered),
                    variant.background(&theme, pressed),
                    "{}",
                    variant.name(),
                );
            }
        }
    }

    /// **Hover is the first visible interaction state in this port**: it moves
    /// `bg`, which is a compared field, on six of the seven variants. `link` is
    /// the exception — its only hover rule is an underline.
    #[test]
    fn hover_moves_the_background_on_every_variant_but_link() {
        for theme in [Theme::LIGHT, Theme::DARK] {
            let hovered = hovering();
            for variant in ALL_VARIANTS {
                let resting = variant.background(&theme, ButtonState::resting());
                let engaged = variant.background(&theme, hovered);
                if matches!(variant, Variant::Link) {
                    assert_eq!(resting, None);
                    assert_eq!(engaged, None);
                    assert!(variant.underlined(hovered));
                } else {
                    assert_ne!(resting, engaged, "{}", variant.name());
                }
            }

            // And only `link` underlines, in either engaged state.
            for variant in ALL_VARIANTS {
                assert_eq!(
                    variant.underlined(hovered),
                    matches!(variant, Variant::Link),
                    "{}",
                    variant.name(),
                );
                assert!(
                    !variant.underlined(ButtonState::resting()),
                    "{}",
                    variant.name(),
                );
            }
        }
    }

    /// The two `outline` variants are the only ones carrying an explicit
    /// **`dark:`** rule — and, separately, **`default` cannot move on the theme
    /// axis at all**, because `theme.css` declares `--primary` and
    /// `--primary-foreground` to the same value in `:root` and in `.dark`.
    ///
    /// That second half is the kind of thing that quietly banks a cell which
    /// cannot fail, so it is asserted rather than left to be noticed: a
    /// `--variant default --theme light|dark` pair is one picture, on the arm
    /// 53 of the 142 live call sites use.
    #[test]
    fn the_dark_overrides_are_the_two_outlines_and_default_is_theme_invariant() {
        // The explicit `dark:bg-input/32` over `bg-popover`.
        for variant in [Variant::Outline, Variant::DestructiveOutline] {
            assert_eq!(
                variant.background(&Theme::LIGHT, ButtonState::resting()),
                Some(Theme::LIGHT.popover),
                "{}",
                variant.name(),
            );
            assert_eq!(
                variant.background(&Theme::DARK, ButtonState::resting()),
                Some(Theme::DARK.input.mix(32.0, Color::TRANSPARENT)),
                "{}",
                variant.name(),
            );
        }

        // Which variants' resting paint the theme axis actually moves.
        let moves: Vec<&str> = ALL_VARIANTS
            .into_iter()
            .filter(|variant| {
                variant.background(&Theme::LIGHT, ButtonState::resting())
                    != variant.background(&Theme::DARK, ButtonState::resting())
                    || variant.foreground(&Theme::LIGHT) != variant.foreground(&Theme::DARK)
            })
            .map(Variant::name)
            .collect();
        assert_eq!(
            moves,
            [
                "destructive",
                "destructive-outline",
                "ghost",
                "link",
                "outline",
                "secondary",
            ],
        );

        // `default` is the one that does not, on either field. `--primary` is
        // `oklch(0.49 0.082 130)` in both tables and `--primary-foreground` is
        // `oklch(0.98 0.027 98)` in both.
        assert_eq!(Theme::LIGHT.primary, Theme::DARK.primary);
        assert_eq!(
            Theme::LIGHT.primary_foreground,
            Theme::DARK.primary_foreground
        );

        // `ghost` and `link` paint no background in either table; what moves
        // them is `text-foreground`.
        for variant in [Variant::Ghost, Variant::Link] {
            assert_eq!(
                variant.background(&Theme::LIGHT, ButtonState::resting()),
                None
            );
            assert_eq!(
                variant.background(&Theme::DARK, ButtonState::resting()),
                None
            );
            assert_ne!(
                variant.foreground(&Theme::LIGHT),
                variant.foreground(&Theme::DARK)
            );
        }
    }

    /// `text-white` is `--color-white`, and `Theme::LIGHT.card` **is** that
    /// token: `theme.css` writes `--card: var(--color-white)` in `:root`.
    ///
    /// The assertion is on the value, because that is what reaches the snapshot,
    /// and on the invariance across the two tables, because that is the part a
    /// reader would doubt.
    #[test]
    fn the_destructive_variants_label_is_white_in_both_themes() {
        for theme in [Theme::LIGHT, Theme::DARK] {
            assert_eq!(Variant::Destructive.foreground(&theme), Theme::LIGHT.card);
            assert_eq!(
                Variant::Destructive.indicator_foreground(&theme),
                Theme::LIGHT.card,
            );
        }
        // White, and not the dark table's `card` — which is the background.
        let white = Theme::LIGHT.card.value();
        assert!((white.l - 1.0).abs() < f32::EPSILON, "{white:?}");
        assert!((white.a - 1.0).abs() < f32::EPSILON, "{white:?}");
        assert!((white.s - 0.0).abs() < f32::EPSILON, "{white:?}");
        assert_ne!(Theme::DARK.card, Theme::LIGHT.card);
    }

    /// `data-loading:text-transparent` is **the only thing a state flag does to a
    /// field the differ can see**, and `loading` forces `disabled` with it.
    #[test]
    fn loading_makes_the_label_transparent_and_forces_disabled() {
        let loading = ButtonState {
            props: Props {
                loading: true,
                ..Props::default()
            },
            ..ButtonState::resting()
        };
        assert!(loading.is_disabled());
        assert!(!ButtonState::resting().is_disabled());
        assert!(
            ButtonState {
                props: Props {
                    disabled: true,
                    ..Props::default()
                },
                ..ButtonState::resting()
            }
            .is_disabled()
        );

        for theme in [Theme::LIGHT, Theme::DARK] {
            for variant in ALL_VARIANTS {
                let mut button = Button::fixture();
                button.variant = variant;
                button.state = loading;
                assert_eq!(
                    button.foreground(&theme),
                    Color::TRANSPARENT,
                    "{}",
                    variant.name()
                );

                button.state = ButtonState::resting();
                assert_eq!(button.foreground(&theme), variant.foreground(&theme));
                assert_ne!(button.foreground(&theme), Color::TRANSPARENT);
            }
        }
    }

    /// The `active` prop replaces the **resting** background and loses to hover
    /// and press — a fact about CSS specificity, not about tailwind-merge.
    #[test]
    fn the_active_prop_tints_the_resting_background_only() {
        let theme = Theme::DARK;
        let tint = theme.accent.mix(ACTIVE_TINT, Color::TRANSPARENT);

        for variant in ALL_VARIANTS {
            let mut button = Button::fixture();
            button.variant = variant;
            button.state = ButtonState {
                props: Props {
                    active: true,
                    ..Props::default()
                },
                ..ButtonState::resting()
            };
            assert_eq!(button.background(&theme), Some(tint), "{}", variant.name());

            // Hovered, the variant's own hover colour wins again.
            button.state.interaction.hovered = true;
            assert_eq!(
                button.background(&theme),
                variant.background(&theme, button.state),
                "{}",
                variant.name(),
            );
            // Which for six of seven is not the tint.
            if !matches!(variant, Variant::Link) {
                assert_ne!(button.background(&theme), Some(tint), "{}", variant.name());
            }
        }
        assert!((ACTIVE_TINT - 20.0).abs() < f32::EPSILON);
    }

    /// The counts behind `live()`, taken over every `<Button` in `web/src/` and
    /// written down so a reader does not have to re-derive them.
    #[test]
    fn six_of_seven_variants_and_six_of_ten_sizes_are_live() {
        let dead_variants: Vec<&str> = ALL_VARIANTS
            .into_iter()
            .filter(|variant| !variant.live())
            .map(Variant::name)
            .collect();
        assert_eq!(dead_variants, ["destructive-outline"]);

        let dead_sizes: Vec<&str> = ALL_SIZES
            .into_iter()
            .filter(|size| !size.live())
            .map(Size::name)
            .collect();
        assert_eq!(dead_sizes, ["icon-lg", "icon-xl", "lg", "xl"]);

        // And the fixture is a live combination, or the default cell would have
        // no reference at all.
        let fixture = Button::fixture();
        assert!(fixture.variant.live() && fixture.size.live());
        assert_eq!(fixture.variant, Variant::Ghost);
        assert_eq!(fixture.size, Size::IconSm);
    }

    /// The fixture is the live app's own cleanest button: `ghost`/`icon`, one
    /// glyph, no label, at rest, on the `sm` side of the breakpoint.
    #[test]
    fn the_fixture_is_the_reference_button() {
        let fixture = Button::fixture();

        assert_eq!(fixture.id, ID_BUTTON);
        assert_eq!(fixture.breakpoint, Breakpoint::Sm);
        assert!(fixture.icon);
        assert_eq!(fixture.label, None);
        assert_eq!(fixture.state, ButtonState::resting());
        assert_eq!(ButtonState::default(), ButtonState::resting());

        // The tab bar's `rounded-sm`, which is what makes this the button a
        // reference can be taken from at all — the two that keep the
        // primitive's own radius are `visible: false`.
        assert_eq!(fixture.class_radius, Some(RadiusClass::Sm));
        assert_eq!(fixture.radius(&Theme::DARK), px(6.0));
        assert_ne!(
            fixture.radius(&Theme::DARK),
            fixture.size.radius(&Theme::DARK)
        );

        // 28×28, which is what the reference measures at `x 1140, y 153`.
        assert_eq!(fixture.size.extent(fixture.breakpoint), px(28.0));
        // And a 16px glyph pulled in by 2px on each side, which is what puts the
        // reference's `<svg>` at `x 6` inside a 28px box: 1px border + a centred
        // 12px outer box in 26px of content is 7, less the 2px margin.
        assert_eq!(fixture.size.icon(fixture.breakpoint), px(16.0));
        assert_eq!(ICON_MARGIN_X, px(-2.0));
    }

    /// **Both declaration lists are empty**, for two different reasons — the
    /// second of which is a stated hole rather than a measurement. See the
    /// module docs.
    #[test]
    fn nothing_on_this_surface_is_declared_content_or_line_sized() {
        assert_eq!(CONTENT_SIZED, [] as [&str; 0]);
        assert_eq!(LINE_SIZED, [] as [&str; 0]);

        // v1.6's own test: a box whose height is *authored* is not line-sized.
        // Every size authors one, in both breakpoints, and none of them equals
        // the line box it contains.
        let theme = Theme::DARK;
        for size in ALL_SIZES {
            for breakpoint in [Breakpoint::Base, Breakpoint::Sm] {
                let step = size.type_step(&theme, breakpoint);
                let line = step.size.0 * 16.0 * step.line_height;
                let box_height = f32::from(size.extent(breakpoint));
                assert!(
                    (box_height - line).abs() > 0.5,
                    "{} {breakpoint:?}: box {box_height} against line {line}",
                    size.name(),
                );
            }
        }
    }

    /// The glyph's box is its **margin box** — the icon less the two
    /// `-mx-0.5` margins — because gpui cannot lay a negative margin out in a
    /// content-sized flex container. See [`ICON_MARGIN_X`]; the control that
    /// measures the defect is `row_layout::button`'s
    /// `a_negative_margin_still_collapses_a_content_sized_flex_container`.
    #[test]
    fn the_glyphs_box_is_the_icon_less_its_two_margins() {
        assert_eq!(ICON_MARGIN_X, px(-2.0));

        for size in ALL_SIZES {
            for breakpoint in [Breakpoint::Base, Breakpoint::Sm] {
                assert_eq!(
                    size.glyph_box(breakpoint),
                    size.icon(breakpoint) - px(4.0),
                    "{} {breakpoint:?}",
                    size.name(),
                );
                // Never negative, or a glyph would consume width instead of
                // contributing it.
                assert!(size.glyph_box(breakpoint) > px(0.0), "{}", size.name());
            }
        }

        // The three glyph scales the class list has: the base list's, `xl`'s
        // and `icon-xs`'s.
        assert_eq!(Size::Default.icon(Breakpoint::Sm), px(16.0));
        assert_eq!(Size::Default.icon(Breakpoint::Base), px(18.0));
        assert_eq!(Size::IconXl.icon(Breakpoint::Sm), px(18.0));
        assert_eq!(Size::IconXs.icon(Breakpoint::Sm), px(14.0));
    }

    /// **A call site's `rounded-*` replaces the variant's, and stops there.**
    ///
    /// The three arms resolve through the sealed tokens rather than through
    /// literals, so the assertion is the arithmetic `theme.css` writes —
    /// `--radius-sm: calc(var(--radius) * 0.6)` and
    /// `--radius-md: calc(var(--radius) * 0.8)` off a single `--radius` — and a
    /// project that moved the base would move all three together rather than
    /// failing here.
    #[test]
    fn a_call_sites_rounded_class_resolves_through_the_token_it_names() {
        for theme in [Theme::LIGHT, Theme::DARK] {
            let base = theme.radius_lg.value();

            // The token each class names, and the compiled multiple it is.
            assert_eq!(RadiusClass::Lg.value(&theme), base);
            assert_eq!(RadiusClass::Md.value(&theme), theme.radius_md.value());
            assert_eq!(RadiusClass::Sm.value(&theme), theme.radius_sm.value());
            assert_eq!(RadiusClass::Md.value(&theme), base * 0.8);
            assert_eq!(RadiusClass::Sm.value(&theme), base * 0.6);

            // Which, at this project's `--radius: 0.625rem`, is 10 / 8 / 6 —
            // and none of the three is Tailwind's stock value for its name.
            assert_eq!(RadiusClass::Lg.value(&theme), px(10.0));
            assert_eq!(RadiusClass::Md.value(&theme), px(8.0));
            assert_eq!(RadiusClass::Sm.value(&theme), px(6.0));

            // Three distinct values, or the option could not move anything.
            let mut seen: Vec<f32> = ALL_RADIUS_CLASSES
                .into_iter()
                .map(|class| f32::from(class.value(&theme)))
                .collect();
            let before = seen.len();
            seen.dedup();
            assert_eq!(seen.len(), before, "{seen:?}");
        }

        // The words the command line takes.
        assert_eq!(
            ALL_RADIUS_CLASSES.map(RadiusClass::name),
            ["sm", "md", "lg"],
        );
    }

    /// The override reaches the **host** and the size variant still decides
    /// everything else — including the `::before` overlay, which is a different
    /// tailwind-merge group.
    #[test]
    fn the_class_moves_the_host_and_leaves_the_overlay_on_the_variants() {
        let theme = Theme::DARK;

        for size in ALL_SIZES {
            let mut button = Button::fixture();
            button.size = size;

            // `None` is the primitive's own.
            button.class_radius = None;
            assert_eq!(
                button.radius(&theme),
                size.radius(&theme),
                "{}",
                size.name()
            );

            for class in ALL_RADIUS_CLASSES {
                button.class_radius = Some(class);
                assert_eq!(
                    button.radius(&theme),
                    class.value(&theme),
                    "{}",
                    size.name()
                );
                // And the overlay is unmoved by it, in every combination.
                assert_eq!(
                    size.overlay_radius(&theme),
                    size.radius(&theme) - BORDER_WIDTH,
                    "{} {}",
                    size.name(),
                    class.name(),
                );
            }
        }

        // The three pairs measured on the live app, host against `::before`.
        let mut tab_bar = Button::fixture();
        assert_eq!(tab_bar.radius(&theme), px(6.0));
        assert_eq!(tab_bar.size.overlay_radius(&theme), px(9.0));

        let mut header = Button::fixture();
        header.size = Size::IconXs;
        header.class_radius = Some(RadiusClass::Lg);
        assert_eq!(header.radius(&theme), px(10.0));
        assert_eq!(header.size.overlay_radius(&theme), px(7.0));

        tab_bar.class_radius = None;
        assert_eq!(tab_bar.radius(&theme), px(10.0));
        assert_eq!(tab_bar.size.overlay_radius(&theme), px(9.0));
    }

    /// The two ids this component writes, distinct — a repeat would make two
    /// anchors one record and the differ would compare whichever arrived last.
    #[test]
    fn the_two_default_ids_are_distinct() {
        assert_ne!(ID_BUTTON, ID_LOADING_INDICATOR);
        assert!(ID_LOADING_INDICATOR.starts_with(ID_BUTTON));
        // The React side writes the same two strings, which is the only thing
        // that makes them anchors rather than names.
        assert_eq!(ID_BUTTON, "button");
        assert_eq!(ID_LOADING_INDICATOR, "button-loading-indicator");
    }

    /// The three label fixtures are button labels rather than the git row's
    /// paths, and they are ordered by length so the content axis means
    /// something.
    #[test]
    fn the_label_fixtures_are_ordered_and_are_not_paths() {
        let short = Label::Short.text();
        let normal = Label::Normal.text();
        let overflow = Label::Overflow.text();

        assert!(short.len() < normal.len());
        assert!(normal.len() < overflow.len());
        for text in [short, normal, overflow] {
            assert!(!text.contains('/'), "{text} is a path, not a label");
            assert!(!text.is_empty());
        }
        assert_eq!(Label::default(), Label::Normal);
    }

    /// The two invisible constants, pinned so that a change to either is a
    /// deliberate one rather than a typo — neither has a gate anywhere else.
    #[test]
    fn the_ring_and_the_disabled_opacity_are_the_compiled_values() {
        // `ring-2` plus `ring-offset-1` is `calc(2px + 1px)`.
        assert_eq!(RING_SPREAD, px(3.0));
        assert_eq!(RING_OFFSET_WIDTH, px(1.0));
        assert_eq!(RING_SPREAD, RING_OFFSET_WIDTH * 3.0);
        // `opacity-64` is `opacity: 64%`, and crucially **not zero** — v1.7's
        // `visible` term fires only at zero, so a disabled button is visible on
        // both sides.
        assert!((DISABLED_OPACITY - 0.64).abs() < f32::EPSILON);
        const { assert!(DISABLED_OPACITY > 0.0) };
    }
}
