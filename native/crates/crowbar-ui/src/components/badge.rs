//! `badge` — a pill of text, and the component `ANCHORS.md` v1.6 was written
//! about.
//!
//! The native half of `web/src/components/ui/badge.tsx`: a `cva()` with a base
//! string, three sizes and eight variants over `@base-ui/react`'s `useRender`.
//! Every value below came out of the app's own `tailwindcss` 4.3.0 with the
//! utility as a candidate — the method `native/MAPPING.md` fixes — and the ones
//! that are not Tailwind's stock values are named as such. See
//! `native/mapping/badge.md`.
//!
//! # Two facts this port did not have to rediscover
//!
//! `git-status-row` anchors a `<Badge variant="warning" size="sm">` as
//! `git-row-badge` and Phase 1 measured it exhaustively. Both facts hold here:
//!
//! * it is **[`CONTENT_SIZED`]** — gpui ceils a text run's max-content width
//!   where `WebKit` keeps the fraction, and a badge's used width *is* that
//!   width (`min-w-*` is a floor, never a stretch: `shrink-0` and no `flex-1`);
//! * it is **not `line_sized`** — every size authors its own height, so the box
//!   is 18/16/20px around a 13.33–16px line box. Declaring it would compare the
//!   two and manufacture a delta on the one anchor this surface has. See
//!   [`LINE_SIZED`].
//!
//! # `border` is 1px on every variant — P3.1's mirror, in a second place
//!
//! The base class list carries a bare `border`, which compiles to
//! `border-width: 1px` unconditionally, plus `border-transparent` which only
//! sets the *colour*. Five of the eight variants leave that colour transparent
//! and still pay the pixel. `ANCHORS.md` v1.1 compares `border.w` **exactly**, so
//! a port that skipped [`BORDER_WIDTH`] because "a warning badge has no visible
//! border" would be wrong on every cell — and wrong about the box, because
//! `box-sizing: border-box` makes the pixel eat into the padding.
//!
//! Measured on the live `agent` badge: `borderTopWidth: "1px"`,
//! `borderTopColor: "oklab(0.49 -0.052709 0.062816 / 0.3)"`.
//!
//! # Half the class list cannot match on the element the app renders
//!
//! `badge.tsx`'s default tag is a **`<span>`**, and no live call site passes
//! `render`. Three groups of rules are therefore dead on every live Badge:
//!
//! | rule | why it cannot match |
//! |---|---|
//! | `[button&,a&]:hover:bg-*` on six variants | the `&` is the badge itself; a `<span>` is neither |
//! | `disabled:pointer-events-none disabled:opacity-64` | `&:disabled` matches form elements only |
//! | `[button&,a&]:pointer-coarse:after:*` | a touch target, behind `@media (pointer: coarse)` |
//!
//! They are **modelled anyway**, through [`Badge::interactive`] and
//! [`BadgeState`], because they are real arms of the primitive — the same call
//! `resizable` made about its grip. What the caption has to say is that the cell
//! has no reference.

use gpui::{
    AnyElement, BoxShadow, Div, FontWeight, ParentElement as _, Pixels, Rems, SharedString,
    Styled as _, div, px, relative,
};

use super::anchor::{AnchorId, AnchorSink};
use super::git_status_row::Breakpoint;
use crate::theme::{Color, Theme};

/// The root anchor: the `<span>` itself. Every other bound on this surface is
/// reported relative to it (`native/oracle/ANCHORS.md` §4).
///
/// Written as a **per-slot default** in `badge.tsx` — a property of the
/// `defaultProps` object rather than a JSX attribute, because `useRender` builds
/// the element from a props bag — and `mergeProps(defaultProps, props)` lets a
/// call site override it. `git-status-file-item.tsx` does exactly that, which is
/// why the six badges in the live git panel are `git-row-badge` and not this.
pub const ID_BADGE: &str = "badge";

/// The anchors on this surface whose boxes size to their own text
/// (`native/oracle/ANCHORS.md` v1.5).
///
/// **The badge itself, and it is the whole surface.** A badge authors no width:
/// `min-w-*` floors it, `shrink-0` stops a flex line squeezing it and nothing
/// grows it, so the used width is the label's max-content width plus the
/// padding and the two border pixels. gpui ceils that run and `WebKit` does not
/// — measured on the live `agent` badge at **44.34** against a native **45**, a
/// 0.66px difference that is entirely the `ceil` and is outside §5's ±0.5.
///
/// The React side's spelling is `data-oracle-content-sized`, and this port puts
/// it on **`badge.tsx`'s own `defaultProps`** rather than at a call site.
/// That is where v1.5 says it belongs — *"Content-sizing is a property of the
/// component"* — and `git-status-file-item.tsx` had already asserted the same
/// thing one call site at a time.
///
/// **The floor case is safe under the same declaration.** When the label is
/// narrower than `min-w-*` the used width is the floor, and every floor here is
/// a whole number of pixels, so `ceil(reference)` is the reference and the rule
/// degenerates to an equality.
pub const CONTENT_SIZED: [&str; 1] = [ID_BADGE];

/// The anchors on this surface whose **box height is their own line box**
/// (`native/oracle/ANCHORS.md` v1.6).
///
/// **Empty, and this component is the reason the rule says "declared, never
/// detected".** v1.6 records the mistake in full: a badge is one line of text in
/// a box, so a detector keyed on "has text and no explicit height" fires on it —
/// and every size authors a height. The live `agent` badge is **18px** around a
/// **16px** line box; the Phase 1 gate's `git-row-badge` is 16 around 13.33.
/// Declaring either would invent a delta on an anchor both engines already
/// agree on.
///
/// An empty list rather than no list, for P2.1's reason: the React side's (also
/// empty) declaration set has to be diffable against this one.
pub const LINE_SIZED: [&str; 0] = [];

/// `--spacing`, Tailwind's stock `0.25rem` at a 16px root.
///
/// `theme.css` does **not** redefine it — checked, because P2.1 found three
/// values in `dropdown-menu` that it *does*. Spelled once so a theme that moved
/// the step would move every length below together.
const SPACING: f32 = 4.0;

/// The base class list's bare `border`, which compiles to `border-width: 1px`
/// with no variant able to change it. See the module docs.
pub const BORDER_WIDTH: Pixels = px(1.0);

/// `gap-1` — between a leading glyph and the label.
pub const GAP: Pixels = px(SPACING);

/// `focus-visible:ring-2` — `0 0 0 calc(2px + var(--tw-ring-offset-width))`, and
/// `focus-visible:ring-offset-1` sets that width to 1px, so the spread is **3**.
///
/// A **box-shadow**, not a border, exactly as `resizable`'s and `button`'s are.
/// `ANCHORS.md` §6 has no field for it.
pub const RING_SPREAD: Pixels = px(3.0);

/// `focus-visible:ring-offset-1` — a second shadow layer, `0 0 0 1px` in
/// `ring-offset-background`, painted **in front of** [`RING_SPREAD`]'s, which is
/// Tailwind's own composite order.
pub const RING_OFFSET_WIDTH: Pixels = px(1.0);

/// `disabled:opacity-64`, which compiles to `opacity: 64%`.
///
/// **Painted and invisible twice over.** `ANCHORS.md` has no opacity field and
/// v1.7's `visible` term fires only at *zero*; and `&:disabled` cannot match the
/// `<span>` every live call site renders.
pub const DISABLED_OPACITY: f32 = 0.64;

/// `[button&,a&]:hover:bg-primary/90`, and the same tint on `destructive` and
/// `secondary`.
const TINT_90: f32 = 90.0;
/// `[button&,a&]:hover:bg-accent/50` on `outline`.
const TINT_50: f32 = 50.0;
/// `dark:[button&,a&]:hover:bg-input/48` on `outline`.
const TINT_48: f32 = 48.0;
/// `dark:bg-input/32` on `outline`.
const TINT_32: f32 = 32.0;
/// The `dark:` tint of the four soft variants: `dark:bg-warning/16` and friends.
const TINT_16: f32 = 16.0;
/// The unprefixed tint of the four soft variants: `bg-warning/8` and friends.
const TINT_8: f32 = 8.0;

/// `border-primary/30` on the review thread's `agent` badge.
const CALL_SITE_BORDER_TINT: f32 = 30.0;
/// `border-border/40` and `text-muted-foreground/60` on the two `Outdated`
/// badges.
const CALL_SITE_BORDER_TINT_OUTDATED: f32 = 40.0;
/// `text-muted-foreground/60`. See [`CallSite::foreground`].
const CALL_SITE_TEXT_TINT_OUTDATED: f32 = 60.0;

/// `[&_svg:not([class*='opacity-'])]:opacity-80` on a leading glyph.
///
/// **Painted and invisible**: `ANCHORS.md` §6 has no opacity field, and 80% does
/// not reach v1.7's zero.
pub const GLYPH_OPACITY: f32 = 0.8;

/// `rounded-[.25rem]` on `size="sm"`, which beats the base list's `rounded-sm`
/// (same tailwind-merge group, written later).
///
/// **A literal with no token behind it.** `theme.css` builds the whole radius
/// scale off one `--radius: 0.625rem`, and its smallest step — `--radius-sm`,
/// `calc(var(--radius) * 0.6)` — is **6px**. `.25rem` is 4, which is not on that
/// scale at all, so this is an arbitrary Tailwind value and reading a token for
/// it would paint a 6px corner where the reference paints 4. Named, with the
/// reason, exactly as `button`'s `TEXT_LG` is.
pub const RADIUS_SM_SIZE: Rems = Rems(0.25);

/// `sm:text-[.625rem]` on `size="sm"` — **10px**, and no crowbar token carries
/// it either.
///
/// `--ui-text-xs` is 0.6875rem (11px) and is a different number. The same
/// statement [`RADIUS_SM_SIZE`] makes, in the type scale.
///
/// **It sets `font-size` and nothing else.** Tailwind's `text-*` utilities carry
/// a paired line-height; an arbitrary `text-[…]` does not. So the line height a
/// `size="sm"` badge gets at the `sm` breakpoint is still the one the *base*
/// `text-xs` set — see [`Size::type_step`], and see `git_status_row`'s archived
/// pair, where the reference reports `font.line_height: 13.33` on a 10px face.
pub const TEXT_SM_SIZE: Rems = Rems(0.625);

/// Tailwind's line-height for `text-base`: `calc(1.5 / 1)`.
const LINE_HEIGHT_BASE: f32 = 1.5;
/// Tailwind's line-height for `text-sm`: `calc(1.25 / 0.875)`.
const LINE_HEIGHT_SM: f32 = 1.25 / 0.875;
/// Tailwind's line-height for `text-xs`: `calc(1 / 0.75)`.
const LINE_HEIGHT_XS: f32 = 1.0 / 0.75;

/// A type step: the size and the unitless line height Tailwind pairs with it.
///
/// The two are separate fields rather than one class because `text-[.625rem]`
/// changes only the first — see [`TEXT_SM_SIZE`].
#[derive(Clone, Copy, Debug, PartialEq)]
pub struct TypeStep {
    /// `font-size`.
    pub size: Rems,
    /// `line-height`, unitless — the ratio, not the resolved px, because gpui
    /// takes a `relative(…)` exactly as CSS takes a number.
    pub line_height: f32,
}

/// `cva`'s `size` union, all three of them.
///
/// **Two are live and one is dead.** Counted over every `<Badge` in `web/src/`:
/// `sm` at two call sites (`git-status-file-item.tsx`, and the dead
/// `diff-review-header.tsx`), `default` at the three in `review-thread-item.tsx`
/// which pass no `size` at all — and **zero** for `lg`. The dead one is rendered
/// anyway, for `resizable`'s reason about the grip.
#[derive(Clone, Copy, Debug, Default, PartialEq, Eq)]
pub enum Size {
    /// `h-5.5 min-w-5.5 px-[calc(--spacing(1)-1px)] text-sm sm:h-4.5 sm:min-w-4.5 sm:text-xs`
    /// — `cva`'s own default, and what all three live `review-thread-item`
    /// badges get.
    #[default]
    Default,
    /// `h-6.5 min-w-6.5 px-[calc(--spacing(1.5)-1px)] text-base sm:h-5.5 sm:min-w-5.5 sm:text-sm`.
    /// **Dead.**
    Lg,
    /// `h-5 min-w-5 rounded-[.25rem] px-[calc(--spacing(1)-1px)] text-xs sm:h-4 sm:min-w-4 sm:text-[.625rem]`
    /// — the Phase 1 gate's badge, and the only size that moves the radius.
    Sm,
}

/// Every size, in `cva`'s declaration order, so a caller cannot drive a subset
/// by accident and a `--help` line cannot advertise one that does not exist.
pub const ALL_SIZES: [Size; 3] = [Size::Default, Size::Lg, Size::Sm];

impl Size {
    /// Its word on the command line and in a caption — `cva`'s own key.
    #[must_use]
    pub const fn name(self) -> &'static str {
        match self {
            Self::Default => "default",
            Self::Lg => "lg",
            Self::Sm => "sm",
        }
    }

    /// Whether any live `<Badge` in `web/src/` passes this size.
    ///
    /// `sm` counts the `git-status-file-item.tsx` call site. It is *also* what
    /// the dead `diff-review-header.tsx` passes, and that one does not count —
    /// see [`Badge::fixture`].
    #[must_use]
    pub const fn live(self) -> bool {
        !matches!(self, Self::Lg)
    }

    /// `h-*` and `sm:h-*`, in `--spacing` steps. Read as `(base, sm)`.
    ///
    /// The `sm:` variant is always exactly one step smaller, which is the whole
    /// of what the breakpoint does to this component's box.
    const fn height_steps(self) -> (f32, f32) {
        match self {
            Self::Default => (5.5, 4.5),
            Self::Lg => (6.5, 5.5),
            Self::Sm => (5.0, 4.0),
        }
    }

    /// The authored height for a viewport on this side of the breakpoint.
    ///
    /// **A call site's own `h-*` does not reach here** — see
    /// [`CallSite::height`] for the tailwind-merge trap, which is that an
    /// unprefixed call-site class loses to the variant's `sm:` one above 640px.
    #[must_use]
    pub fn height(self, breakpoint: Breakpoint) -> Pixels {
        let (base, small) = self.height_steps();
        px(SPACING
            * match breakpoint {
                Breakpoint::Base => base,
                Breakpoint::Sm => small,
            })
    }

    /// `min-w-*` / `sm:min-w-*`, which is always the same step as the height —
    /// so an empty badge is a circle and a one-character badge is a disc.
    ///
    /// No live call site writes a `min-w-*`, so unlike the height this one is
    /// never merged over.
    #[must_use]
    pub fn min_width(self, breakpoint: Breakpoint) -> Pixels {
        self.height(breakpoint)
    }

    /// `px-[calc(--spacing(n)-1px)]`.
    ///
    /// The `-1px` is the base class list's `border` being paid for out of the
    /// padding rather than added to the box: `box-sizing: border-box` is on, so
    /// `px-1` plus a 1px border would be a 5px gap between the border box and
    /// the text where the design asks for 4. It has no `sm:` variant.
    #[must_use]
    pub fn padding_x(self) -> Pixels {
        let steps = match self {
            Self::Default | Self::Sm => 1.0,
            Self::Lg => 1.5,
        };
        px(SPACING * steps - f32::from(BORDER_WIDTH))
    }

    /// `rounded-sm` from the base class list, which `sm` overrides with
    /// `rounded-[.25rem]`.
    ///
    /// **`rounded-sm` is not Tailwind's stock 2px.** `theme.css` sets
    /// `--radius: 0.625rem` and `--radius-sm: calc(var(--radius) * 0.6)`, so it
    /// is **6px** — confirmed live on the `agent` badge at
    /// `borderTopLeftRadius: "6px"`. The `sm` size's override is an arbitrary
    /// value and not on that scale at all; see [`RADIUS_SM_SIZE`].
    #[must_use]
    pub fn radius(self, theme: &Theme) -> Pixels {
        match self {
            Self::Sm => rems_to_px(RADIUS_SM_SIZE),
            Self::Default | Self::Lg => theme.radius_sm.value(),
        }
    }

    /// The type step, before a call site has had its say.
    ///
    /// The token each step reads is the one carrying its number — the
    /// `--ui-text-*` trade `native/MAPPING.md` states once — except
    /// [`TEXT_SM_SIZE`], which no token carries.
    ///
    /// **`sm:text-[.625rem]` keeps the base step's line height**, because an
    /// arbitrary `text-[…]` sets `font-size` alone while `text-xs` sets both.
    /// That is why the archived `git-row-badge` reference reports a 13.33px line
    /// box on a 10px face: 10 × (1 / 0.75).
    #[must_use]
    pub fn type_step(self, theme: &Theme, breakpoint: Breakpoint) -> TypeStep {
        match (self, breakpoint) {
            // `text-sm` = 0.875rem = `--ui-text-base`.
            (Self::Default, Breakpoint::Base) | (Self::Lg, Breakpoint::Sm) => TypeStep {
                size: theme.ui_text_base.value(),
                line_height: LINE_HEIGHT_SM,
            },
            // `text-base` = 1rem = `--ui-text-lg`.
            (Self::Lg, Breakpoint::Base) => TypeStep {
                size: theme.ui_text_lg.value(),
                line_height: LINE_HEIGHT_BASE,
            },
            // `sm:text-[.625rem]` over `text-xs`: the face moves, the ratio does
            // not.
            (Self::Sm, Breakpoint::Sm) => TypeStep {
                size: TEXT_SM_SIZE,
                line_height: LINE_HEIGHT_XS,
            },
            // `text-xs` = 0.75rem = `--ui-text-sm`.
            (Self::Default, Breakpoint::Sm) | (Self::Sm, Breakpoint::Base) => TypeStep {
                size: theme.ui_text_sm.value(),
                line_height: LINE_HEIGHT_XS,
            },
        }
    }

    /// `[&_svg:not([class*='size-'])]:size-3.5 sm:…size-3` — a leading glyph.
    #[must_use]
    pub fn glyph(self, breakpoint: Breakpoint) -> Pixels {
        px(SPACING
            * match breakpoint {
                Breakpoint::Base => 3.5,
                Breakpoint::Sm => 3.0,
            })
    }
}

/// `cva`'s `variant` union, all eight of them.
///
/// **Three are live and five are dead.** Counted over every `<Badge` in
/// `web/src/`: `outline` at the three in `review-thread-item.tsx`, `warning` at
/// `git-status-file-item.tsx`, `secondary` at `diff-review-header.tsx` — and
/// **zero** for `default`, `destructive`, `error`, `info` and `success`.
///
/// `secondary`'s only call site is inside `DiffReviewHeader`, which is itself
/// **dead code**: `git grep DiffReviewHeader` finds its own definition and its
/// unit test and nothing else. It is counted live here anyway, because the
/// component exists and would render it — but the caption has to say the
/// reference cannot.
#[derive(Clone, Copy, Debug, Default, PartialEq, Eq)]
pub enum Variant {
    /// `bg-primary text-primary-foreground`. `cva`'s own default. **Dead.**
    #[default]
    Default,
    /// `bg-destructive text-white`. **Dead.**
    Destructive,
    /// `bg-destructive/8 text-destructive-foreground dark:bg-destructive/16`.
    /// **Dead.**
    Error,
    /// `bg-info/8 text-info-foreground dark:bg-info/16`. **Dead.**
    Info,
    /// `border-input bg-background text-foreground dark:bg-input/32` — the only
    /// variant that paints a visible border, and the one all three live
    /// `review-thread-item` badges use.
    Outline,
    /// `bg-secondary text-secondary-foreground`.
    Secondary,
    /// `bg-success/8 text-success-foreground dark:bg-success/16`. **Dead.**
    Success,
    /// `bg-warning/8 text-warning-foreground dark:bg-warning/16` — the Phase 1
    /// gate's badge.
    Warning,
}

/// Every variant, in `cva`'s declaration order.
pub const ALL_VARIANTS: [Variant; 8] = [
    Variant::Default,
    Variant::Destructive,
    Variant::Error,
    Variant::Info,
    Variant::Outline,
    Variant::Secondary,
    Variant::Success,
    Variant::Warning,
];

impl Variant {
    /// Its word on the command line and in a caption — `cva`'s own key.
    #[must_use]
    pub const fn name(self) -> &'static str {
        match self {
            Self::Default => "default",
            Self::Destructive => "destructive",
            Self::Error => "error",
            Self::Info => "info",
            Self::Outline => "outline",
            Self::Secondary => "secondary",
            Self::Success => "success",
            Self::Warning => "warning",
        }
    }

    /// Whether any live `<Badge` in `web/src/` passes this variant.
    #[must_use]
    pub const fn live(self) -> bool {
        matches!(self, Self::Outline | Self::Secondary | Self::Warning)
    }

    /// The border **colour**. The border's *width* is [`BORDER_WIDTH`] on every
    /// variant there is — see the module docs.
    ///
    /// `outline` is the only variant that names one; the other seven inherit
    /// the base list's `border-transparent`, which is still a one-pixel border.
    #[must_use]
    pub fn border(self, theme: &Theme) -> Color {
        match self {
            Self::Outline => theme.input,
            _ => Color::TRANSPARENT,
        }
    }

    /// The background.
    ///
    /// Every variant paints one, which is why this returns a `Color` rather
    /// than `button`'s `Option<Color>`: there is no `link`-shaped arm here.
    ///
    /// The four soft variants layer `dark:bg-*/16` over `bg-*/8`; `outline`
    /// layers `dark:bg-input/32` over `bg-background`. Both are the same
    /// switch that selects the token table, which is why [`is_dark`] can answer
    /// it without an `Appearance` argument.
    #[must_use]
    pub fn background(self, theme: &Theme, state: BadgeState, interactive: bool) -> Color {
        let dark = is_dark(theme);
        // `[button&,a&]:hover:bg-*` — only when `render` made this a button or
        // an anchor. See the module docs.
        let engaged = interactive && state.hovered;
        match self {
            Self::Default if engaged => theme.primary.mix(TINT_90, Color::TRANSPARENT),
            Self::Default => theme.primary,
            Self::Destructive if engaged => theme.destructive.mix(TINT_90, Color::TRANSPARENT),
            Self::Destructive => theme.destructive,
            Self::Secondary if engaged => theme.secondary.mix(TINT_90, Color::TRANSPARENT),
            Self::Secondary => theme.secondary,
            Self::Outline if engaged && dark => theme.input.mix(TINT_48, Color::TRANSPARENT),
            Self::Outline if engaged => theme.accent.mix(TINT_50, Color::TRANSPARENT),
            Self::Outline if dark => theme.input.mix(TINT_32, Color::TRANSPARENT),
            Self::Outline => theme.background,
            Self::Error => theme.destructive.mix(soft_tint(dark), Color::TRANSPARENT),
            Self::Info => theme.info.mix(soft_tint(dark), Color::TRANSPARENT),
            Self::Success => theme.success.mix(soft_tint(dark), Color::TRANSPARENT),
            Self::Warning => theme.warning.mix(soft_tint(dark), Color::TRANSPARENT),
        }
    }

    /// The text colour.
    ///
    /// **`destructive` is the awkward one**, exactly as it is on `button`:
    /// `text-white` compiles to `color: var(--color-white)`, which is Tailwind's
    /// own token and not one `Theme` carries a field for. The port reads
    /// `Theme::LIGHT.card`, which is not a coincidence of value but the same
    /// declaration — `theme.css` writes `--card: var(--color-white)` in
    /// `:root`, so the light table's `card` **is** `--color-white`. Deliberately
    /// not `theme.card`, which in dark is the background.
    #[must_use]
    pub fn foreground(self, theme: &Theme) -> Color {
        match self {
            Self::Default => theme.primary_foreground,
            Self::Destructive => Theme::LIGHT.card,
            Self::Error => theme.destructive_foreground,
            Self::Info => theme.info_foreground,
            Self::Outline => theme.foreground,
            Self::Secondary => theme.secondary_foreground,
            Self::Success => theme.success_foreground,
            Self::Warning => theme.warning_foreground,
        }
    }
}

/// The tint the four soft variants paint at, which is the only thing `dark:`
/// changes about them.
const fn soft_tint(dark: bool) -> f32 {
    if dark { TINT_16 } else { TINT_8 }
}

/// The `className` a **call site** merges over the variant's own.
///
/// # Why this is a parameter, and where the line is
///
/// `badge.tsx` composes `cn(badgeVariants({ className, size, variant }))`, so a
/// call site's `className` is merged over the variant's by tailwind-merge —
/// **the call site is half the component**, which is the line P3.1 drew for
/// `button`'s `--class-radius` and the same line applies here:
///
/// * **forbidden** — a knob that hands the port the reference's *output*;
/// * **correct** — a knob that supplies the same *input* both engines then
///   resolve independently.
///
/// So every arm below names the **classes** the call site literally writes, and
/// the pixels come out of the sealed tokens those classes name. There is
/// deliberately no `--padding 4`.
///
/// Only one class bundle exists in the app, at three call sites: the three
/// `review-thread-item.tsx` badges all write `h-4 px-1 text-xs` plus a border
/// colour and a text colour, and the other two live call sites write no
/// `className` at all.
#[derive(Clone, Copy, Debug, Default, PartialEq, Eq)]
pub enum CallSite {
    /// No `className` — the variant's own classes, unmerged.
    ///
    /// `git-status-file-item.tsx` (`warning`/`sm`) and the dead
    /// `diff-review-header.tsx` (`secondary`/`sm`).
    #[default]
    None,
    /// `h-4 border-primary/30 px-1 text-xs text-primary` — the `agent` badge in
    /// `review-thread-item.tsx`, rendered when a review message carries
    /// `isAgent`.
    ///
    /// **The only live Badge that keeps the primitive's own `data-oracle-id`**,
    /// and therefore the only reference this surface has. See [`Badge::fixture`].
    Agent,
    /// `h-4 border-border/40 px-1 text-xs text-muted-foreground/60` — the two
    /// `Outdated` badges in `review-thread-item.tsx`.
    ///
    /// **Unreachable.** Both are gated on the `isOutdated` prop, and
    /// `use-review-annotations.tsx` — the component's only call site — never
    /// passes it. A real arm of a real component with no live rendering, which
    /// is `button`'s `active` prop in a second place.
    Outdated,
}

/// Every call-site bundle, in the order they appear in `review-thread-item.tsx`.
pub const ALL_CALL_SITES: [CallSite; 3] = [CallSite::None, CallSite::Agent, CallSite::Outdated];

impl CallSite {
    /// Its word on the command line and in a caption.
    #[must_use]
    pub const fn name(self) -> &'static str {
        match self {
            Self::None => "none",
            Self::Agent => "agent",
            Self::Outdated => "outdated",
        }
    }

    /// Whether this bundle has a live rendering in the app.
    #[must_use]
    pub const fn live(self) -> bool {
        !matches!(self, Self::Outdated)
    }

    /// `h-4` — **and it is dead above the `sm` breakpoint.**
    ///
    /// This is the trap of the whole component. `h-4` and the variant's
    /// `sm:h-4.5` are the same tailwind-merge group but *different modifiers*,
    /// so both survive the merge and both reach the stylesheet; Tailwind emits
    /// `sm:`-prefixed rules **after** unprefixed ones, so at ≥640px the
    /// variant's wins and the call site's 16px never paints. What the call site
    /// *does* remove is the variant's unprefixed `h-5.5`, which is in the same
    /// group with the same modifier.
    ///
    /// Measured on the live `agent` badge at a 1714px viewport:
    /// `getComputedStyle().height` is **18px** — `sm:h-4.5` — not the 16 the
    /// class list reads as.
    #[must_use]
    pub fn height(self) -> Option<Pixels> {
        match self {
            Self::None => None,
            Self::Agent | Self::Outdated => Some(px(SPACING * 4.0)),
        }
    }

    /// `px-1` — 4px, and unlike the height this one **wins in every cell**,
    /// because the variant's `px-[calc(--spacing(1)-1px)]` is unprefixed too
    /// and tailwind-merge drops it.
    ///
    /// So the call site pays the border pixel twice: 4px of padding *plus* the
    /// base list's 1px border, where the variant's own arithmetic subtracts it.
    /// Measured live: `paddingLeft: "4px"`.
    #[must_use]
    pub fn padding_x(self) -> Option<Pixels> {
        match self {
            Self::None => None,
            Self::Agent | Self::Outdated => Some(px(SPACING)),
        }
    }

    /// `text-xs` — 0.75rem, with Tailwind's paired `calc(1 / 0.75)`.
    ///
    /// Unprefixed, so it behaves like the height: it removes the variant's
    /// unprefixed step and loses to the variant's `sm:` one. On `size="default"`
    /// that is invisible — `sm:text-xs` is the same 0.75rem — and on
    /// `size="sm"` it would lose to `sm:text-[.625rem]`. No live call site pairs
    /// this bundle with `size="sm"`.
    #[must_use]
    pub fn type_step(self, theme: &Theme) -> Option<TypeStep> {
        match self {
            Self::None => None,
            Self::Agent | Self::Outdated => Some(TypeStep {
                size: theme.ui_text_sm.value(),
                line_height: LINE_HEIGHT_XS,
            }),
        }
    }

    /// `border-primary/30` / `border-border/40`.
    #[must_use]
    pub fn border(self, theme: &Theme) -> Option<Color> {
        match self {
            Self::None => None,
            Self::Agent => Some(theme.primary.mix(CALL_SITE_BORDER_TINT, Color::TRANSPARENT)),
            Self::Outdated => Some(
                theme
                    .border
                    .mix(CALL_SITE_BORDER_TINT_OUTDATED, Color::TRANSPARENT),
            ),
        }
    }

    /// `text-primary` / `text-muted-foreground/60`.
    #[must_use]
    pub fn foreground(self, theme: &Theme) -> Option<Color> {
        match self {
            Self::None => None,
            Self::Agent => Some(theme.primary),
            Self::Outdated => Some(
                theme
                    .muted_foreground
                    .mix(CALL_SITE_TEXT_TINT_OUTDATED, Color::TRANSPARENT),
            ),
        }
    }
}

/// Whether a `dark:` Tailwind variant is in force.
///
/// A local copy of the one `git_status_row`, `dropdown_menu` and `button` each
/// carry, kept local deliberately: the components are ported independently and a
/// shared helper would make one surface's diff reach into another's file.
fn is_dark(theme: &Theme) -> bool {
    *theme == Theme::DARK
}

/// gpui's rem size, which is what a `Rems` resolves against.
///
/// Named because two of this component's values are authored in rems and one
/// consumer — [`Size::radius`] — has to hand gpui a `Pixels`. 16 is gpui's own
/// default and is the root font size the reference runs at.
const REM_SIZE: f32 = 16.0;

/// A `Rems` in logical pixels at [`REM_SIZE`].
fn rems_to_px(value: Rems) -> Pixels {
    px(value.0 * REM_SIZE)
}

/// The three CSS pseudo-classes `badge.tsx` has a rule for.
///
/// **Parameters, not gpui `.hover(…)` refinements** — `ANCHORS.md` §6 makes
/// runtime interaction state invisible to the extractor, so a component that
/// expressed its states that way would report its resting paint in every cell.
#[derive(Clone, Copy, Debug, Default, PartialEq, Eq)]
pub struct BadgeState {
    /// `[button&,a&]:hover:bg-*`. Moves `bg` on four variants — but only when
    /// [`Badge::interactive`], and no live call site makes it so.
    pub hovered: bool,
    /// `:focus-visible`, whose only rules are `ring-2`, `ring-offset-1` and
    /// `outline-none`: two box-shadows and an outline, and `ANCHORS.md` §6 has
    /// a field for neither.
    pub focused: bool,
    /// `:disabled`, whose rules are `pointer-events-none` (not a visual
    /// property) and `opacity-64` (no field, and non-zero so v1.7's `visible`
    /// term does not fire). Dead on a `<span>` as well.
    pub disabled: bool,
}

/// Which of the three §8.3 content lengths the badge shows.
///
/// A type of its own rather than `ContentLength` reused, for `button`'s reason:
/// the git row's fixture strings are **paths** and a badge's label is a word.
#[derive(Clone, Copy, Debug, Default, PartialEq, Eq)]
pub enum Label {
    /// One character — the case `min-w-*` exists for, where the floor decides
    /// the width instead of the text.
    Short,
    /// The live `agent` badge's own word.
    #[default]
    Normal,
    /// The Phase 1 gate badge's word, which is the longest the app renders.
    ///
    /// It cannot **clip**: the base class list has `whitespace-nowrap` and no
    /// `overflow-hidden`, and `shrink-0` stops a flex line squeezing it, so a
    /// long label makes the badge wider rather than truncating it.
    Overflow,
}

impl Label {
    /// The string this length shows.
    #[must_use]
    pub const fn text(self) -> &'static str {
        match self {
            Self::Short => "3",
            Self::Normal => "agent",
            Self::Overflow => "uncommitted",
        }
    }
}

/// One `<Badge>`.
#[derive(Clone, Debug, PartialEq)]
pub struct Badge {
    /// The anchor the `<span>` carries.
    ///
    /// Authored rather than fixed, because `badge.tsx` writes [`ID_BADGE`] into
    /// its `defaultProps` *before* `mergeProps` folds a call site's props over
    /// it — and one call site overrides it. `git-status-file-item.tsx` names
    /// its badge `git-row-badge`, so the six badges in the live git panel do
    /// **not** answer to this surface's root.
    pub id: SharedString,
    /// `variant`.
    pub variant: Variant,
    /// `size`.
    pub size: Size,
    /// Which side of the `sm` breakpoint the **viewport** is on.
    ///
    /// A parameter for the reason `git_status_row`'s is: a media query asks
    /// about the window, not about the box this component is drawn in, and gpui
    /// has no media queries at all. It is the axis that does the most work on
    /// this component — every size moves its height, its floor and its face.
    pub breakpoint: Breakpoint,
    /// A call site's own `className`. See [`CallSite`].
    pub call_site: CallSite,
    /// The label.
    ///
    /// Not an `Option`: `empty` is [`Badge::label`] set to `None`, which is the
    /// §8.3 flag and is handled at the surface. A badge with no label is the
    /// picture `min-w-*` exists for.
    pub label: Option<Label>,
    /// Whether a leading glyph is rendered.
    ///
    /// An **empty box**, as every icon in this port is. **No live call site
    /// passes a child other than text**, so this has no reference.
    pub glyph: bool,
    /// Whether `render` made this a `<button>` or an `<a>`, which is what every
    /// `[button&,a&]:` rule selects.
    ///
    /// **No live call site passes `render`**, so every live Badge is a `<span>`
    /// and the whole hover group is dead. Modelled because the rules are real.
    pub interactive: bool,
    /// The visual state.
    pub state: BadgeState,
}

impl Badge {
    /// The badge a reference can actually be taken from: `variant="outline"`
    /// `size="default"` with `review-thread-item.tsx`'s `agent` bundle, one
    /// label, no glyph, a `<span>`, at rest.
    ///
    /// Read off the live app rather than chosen, and the choice was forced.
    /// There are five `<Badge` call sites in `web/src/` and only one of them
    /// renders an element carrying the primitive's own `data-oracle-id`:
    ///
    /// | call site | id | reachable |
    /// |---|---|---|
    /// | `git-status-file-item.tsx` | **`git-row-badge`** — overridden | yes, ×6 |
    /// | `diff-review-header.tsx` | `badge` | **no** — `DiffReviewHeader` is dead code |
    /// | `review-thread-item.tsx` ×2, `Outdated` | `badge` | **no** — `isOutdated` is never passed |
    /// | `review-thread-item.tsx`, `agent` | `badge` | **yes**, on a review message with `isAgent` |
    ///
    /// So the fixture is the `agent` badge, measured at
    /// `44.34 × 18, radius 6, border 1px` on a 1714px viewport.
    #[must_use]
    pub fn fixture() -> Self {
        Self {
            id: SharedString::new_static(ID_BADGE),
            variant: Variant::Outline,
            size: Size::Default,
            breakpoint: Breakpoint::Sm,
            call_site: CallSite::Agent,
            label: Some(Label::Normal),
            glyph: false,
            interactive: false,
            state: BadgeState::default(),
        }
    }

    /// The height the badge is drawn at.
    ///
    /// A call site's unprefixed `h-*` wins **below** the breakpoint and loses
    /// **above** it, which is [`CallSite::height`]'s trap stated as arithmetic.
    #[must_use]
    pub fn height(&self) -> Pixels {
        match (self.breakpoint, self.call_site.height()) {
            (Breakpoint::Base, Some(height)) => height,
            _ => self.size.height(self.breakpoint),
        }
    }

    /// The horizontal padding: a call site's `px-1` where there is one, and the
    /// size variant's own where there is not. Neither has a `sm:` form, so the
    /// breakpoint does not reach this.
    #[must_use]
    pub fn padding_x(&self) -> Pixels {
        self.call_site
            .padding_x()
            .unwrap_or_else(|| self.size.padding_x())
    }

    /// The type step, after a call site has had its say — same shape as
    /// [`Badge::height`], and for the same reason.
    #[must_use]
    pub fn type_step(&self, theme: &Theme) -> TypeStep {
        match (self.breakpoint, self.call_site.type_step(theme)) {
            (Breakpoint::Base, Some(step)) => step,
            _ => self.size.type_step(theme, self.breakpoint),
        }
    }

    /// The border colour: a call site's where there is one, the variant's where
    /// there is not. Both are unprefixed, so the call site wins in every cell.
    #[must_use]
    pub fn border_color(&self, theme: &Theme) -> Color {
        self.call_site
            .border(theme)
            .unwrap_or_else(|| self.variant.border(theme))
    }

    /// The text colour, resolved the same way.
    #[must_use]
    pub fn foreground(&self, theme: &Theme) -> Color {
        self.call_site
            .foreground(theme)
            .unwrap_or_else(|| self.variant.foreground(theme))
    }

    /// Renders the badge, opting the contract anchor into `anchors`.
    ///
    /// The label goes through [`AnchorSink::boxed_text`] rather than
    /// [`AnchorSink::text_half`] when there is no glyph, because the run *is*
    /// the box's only content — the arrangement `git-row-badge` already uses.
    /// With a glyph the placement becomes the caller's, so that `[glyph, label]`
    /// keeps its DOM order instead of the run being appended last.
    ///
    /// `whitespace-nowrap` is the rare class that ports to something and
    /// matters: gpui only computes a wrap width when `white_space` is `Normal`,
    /// so `.whitespace_nowrap()` is what makes a long label shape on one line
    /// here as it does in `WebKit`. A wrapped run is outside what the contract
    /// can compare at all.
    #[must_use]
    pub fn render(&self, theme: &Theme, anchors: &dyn AnchorSink) -> AnyElement {
        let id = AnchorId::new(self.id.clone()).content_sized();
        let shell = self.shell(theme);

        match (self.glyph, self.label) {
            (false, Some(label)) => {
                anchors.boxed_text(id, shell, SharedString::new_static(label.text()))
            }
            (true, Some(label)) => {
                let run = anchors.text_half(&id, SharedString::new_static(label.text()));
                anchors.boxed(id, shell.child(self.glyph_box(theme)).child(run))
            }
            (true, None) => anchors.boxed(id, shell.child(self.glyph_box(theme))),
            (false, None) => anchors.boxed(id, shell),
        }
    }

    /// The `<span>`'s own box.
    ///
    /// `inline-flex` becomes `.flex()`, and that is not a loss: gpui has no
    /// inline flow at all, and the reference's own computed `display` is
    /// **`flex`** rather than `inline-flex` — measured live, because CSS
    /// blockifies the display of a flex item and every live Badge is one.
    ///
    /// The font family is named explicitly rather than left to inherit, for
    /// `git_status_row`'s reason: `ANCHORS.md` v1.2 makes `font.family` the
    /// *declared* first family, and a style inheriting macOS's `.SystemUIFont`
    /// reports a literal string the DOM will never produce. The reference says
    /// `CalSansUI`.
    fn shell(&self, theme: &Theme) -> Div {
        let step = self.type_step(theme);
        let family = theme.font_sans.primary().unwrap_or("sans-serif");
        let mut element = div()
            .font_family(family)
            .relative()
            .flex()
            .flex_shrink_0()
            .items_center()
            .justify_center()
            .gap(GAP)
            .whitespace_nowrap()
            .h(self.height())
            .min_w(self.size.min_width(self.breakpoint))
            .px(self.padding_x())
            .rounded(self.size.radius(theme))
            .border_1()
            .border_color(self.border_color(theme))
            .bg(self.variant.background(theme, self.state, self.interactive))
            .text_size(step.size)
            .line_height(relative(step.line_height))
            .font_weight(FontWeight::MEDIUM)
            .text_color(self.foreground(theme));

        if self.state.disabled && self.interactive {
            element = element.opacity(DISABLED_OPACITY);
        }
        if self.state.focused {
            element = ring(element, theme);
        }
        element
    }

    /// A call site's leading `<svg>`, as an empty box.
    ///
    /// The same call every component since `git_status_row` has made about
    /// icons: the glyph is an SVG a call site chooses, there is no native
    /// equivalent, and drawing a substitute would put a shape on screen for the
    /// oracle to converge on.
    ///
    /// `[&_svg]:shrink-0` and `[&_svg:not([class*='opacity-'])]:opacity-80` both
    /// land here; `[&_svg]:pointer-events-none` is not a visual property.
    fn glyph_box(&self, theme: &Theme) -> Div {
        let extent = self.size.glyph(self.breakpoint);
        div()
            .flex_shrink_0()
            .w(extent)
            .h(extent)
            .opacity(GLYPH_OPACITY)
            .text_color(self.foreground(theme))
    }
}

/// Adds `focus-visible:ring-2 ring-offset-1`'s two shadow layers in front of
/// whatever shadows `element` already carries.
///
/// A local copy of the one `dropdown_menu`, `resizable` and `button` each carry,
/// for their reason: the components are ported independently and a shared helper
/// would make one surface's diff reach into another's file. Tailwind's composite
/// puts the offset **in front of** the ring, so the two are inserted in that
/// order.
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
        ALL_CALL_SITES, ALL_SIZES, ALL_VARIANTS, Badge, BadgeState, Breakpoint, CONTENT_SIZED,
        CallSite, ID_BADGE, LINE_SIZED, Label, RADIUS_SM_SIZE, Size, TEXT_SM_SIZE, Variant,
        rems_to_px,
    };
    use crate::theme::Theme;
    use gpui::px;

    /// v1.5. The badge's box **is** its text's max-content width, and the
    /// declaration is the whole surface.
    #[test]
    fn the_badge_is_the_only_content_sized_anchor() {
        assert_eq!(CONTENT_SIZED, [ID_BADGE]);
        assert!(
            super::AnchorId::new(ID_BADGE).content_sized().content_sized,
            "the declaration has to survive the builder"
        );
    }

    /// v1.6, and the trap the rule was written about. A badge paints text, so it
    /// reads like the line-sized case; its height is authored by every one of the
    /// three sizes, so it is not. Held as an assertion because the mistake was
    /// made once already, on `git-row-badge`.
    #[test]
    fn nothing_here_is_line_sized_because_every_size_authors_a_height() {
        assert!(LINE_SIZED.is_empty());
        assert!(!LINE_SIZED.contains(&ID_BADGE));

        let theme = Theme::DARK;
        for size in ALL_SIZES {
            for breakpoint in [Breakpoint::Base, Breakpoint::Sm] {
                let authored = size.height(breakpoint);
                let line = size.type_step(&theme, breakpoint);
                let line_box = px(f32::from(line.size.to_pixels(px(16.0))) * line.line_height);
                assert!(
                    (authored - line_box).abs() > px(0.5),
                    "{}/{breakpoint:?}: box {authored:?} against line {line_box:?} — if these \
                     ever agree, the v1.6 argument has to be re-made",
                    size.name(),
                );
            }
        }
    }

    /// The measured pair from the live `agent` badge, at a 1714px viewport:
    /// 18px tall on a 12px face, 4px of padding either side, a 6px radius and a
    /// one-pixel border.
    ///
    /// The height is the assertion that matters: the call site writes `h-4`,
    /// which reads as 16, and the variant's `sm:h-4.5` beats it above 640px.
    #[test]
    fn the_fixture_is_the_live_agent_badge() {
        let theme = Theme::DARK;
        let badge = Badge::fixture();

        assert_eq!(badge.id, ID_BADGE);
        assert_eq!(badge.variant, Variant::Outline);
        assert_eq!(badge.size, Size::Default);
        assert_eq!(badge.call_site, CallSite::Agent);
        assert_eq!(badge.breakpoint, Breakpoint::Sm);
        assert!(!badge.interactive, "every live Badge renders as a <span>");

        assert_eq!(
            badge.height(),
            px(18.0),
            "sm:h-4.5, not the call site's h-4"
        );
        assert_eq!(badge.padding_x(), px(4.0), "px-1 beats the variant's 3px");
        assert_eq!(badge.size.min_width(Breakpoint::Sm), px(18.0));
        assert_eq!(badge.size.radius(&theme), px(6.0), "rounded-sm is 6, not 2");
        assert_eq!(super::BORDER_WIDTH, px(1.0));

        let step = badge.type_step(&theme);
        assert_eq!(step.size.to_pixels(px(16.0)), px(12.0));
        assert!((f32::from(step.size.to_pixels(px(16.0))) * step.line_height - 16.0).abs() < 0.01);
        assert_eq!(badge.label.map(Label::text), Some("agent"));
    }

    /// The trap, from both sides of the breakpoint. Below 640px the call site's
    /// unprefixed `h-4` is the only height left standing; at or above it the
    /// variant's `sm:h-4.5` is emitted later and wins.
    #[test]
    fn a_call_sites_height_wins_below_the_breakpoint_and_loses_above_it() {
        let mut badge = Badge::fixture();

        badge.breakpoint = Breakpoint::Base;
        assert_eq!(badge.height(), px(16.0), "h-4");

        badge.breakpoint = Breakpoint::Sm;
        assert_eq!(badge.height(), px(18.0), "sm:h-4.5");

        // Without the bundle the variant's own unprefixed height survives.
        badge.call_site = CallSite::None;
        badge.breakpoint = Breakpoint::Base;
        assert_eq!(badge.height(), px(22.0), "h-5.5");
    }

    /// The padding is the other half of the same merge and behaves the opposite
    /// way: both classes are unprefixed, so tailwind-merge drops the variant's
    /// and the call site's 4px wins at every viewport.
    #[test]
    fn a_call_sites_padding_wins_at_every_viewport() {
        let mut badge = Badge::fixture();
        for breakpoint in [Breakpoint::Base, Breakpoint::Sm] {
            badge.breakpoint = breakpoint;
            assert_eq!(badge.padding_x(), px(4.0));
        }

        badge.call_site = CallSite::None;
        assert_eq!(badge.padding_x(), px(3.0), "calc(--spacing(1) - 1px)");
        badge.size = Size::Lg;
        assert_eq!(badge.padding_x(), px(5.0), "calc(--spacing(1.5) - 1px)");
    }

    /// The Phase 1 gate's badge, re-derived here rather than taken on trust:
    /// `size="sm"` at the `sm` breakpoint is 16px tall on a **10px** face whose
    /// line box is 13.33, because `sm:text-[.625rem]` moves the size and leaves
    /// `text-xs`'s ratio alone.
    #[test]
    fn the_sm_size_keeps_the_base_steps_line_height() {
        let theme = Theme::DARK;
        let step = Size::Sm.type_step(&theme, Breakpoint::Sm);

        assert_eq!(step.size, TEXT_SM_SIZE);
        assert_eq!(step.size.to_pixels(px(16.0)), px(10.0));
        let line = f32::from(step.size.to_pixels(px(16.0))) * step.line_height;
        assert!((line - 13.33).abs() < 0.01, "{line}");
        assert_eq!(Size::Sm.height(Breakpoint::Sm), px(16.0));
        // And the radius the `sm` size swaps in is an arbitrary value, not the
        // token scale's smallest step.
        assert_eq!(rems_to_px(RADIUS_SM_SIZE), px(4.0));
        assert_ne!(Size::Sm.radius(&theme), theme.radius_sm.value());
    }

    /// Every variant pays the border pixel, including the seven whose colour is
    /// transparent. `ANCHORS.md` v1.1 compares `border.w` exactly, so this is the
    /// field a "no visible border" shortcut would get wrong on every cell.
    #[test]
    fn every_variant_is_one_pixel_bordered_and_only_outline_names_a_colour() {
        let theme = Theme::DARK;
        for variant in ALL_VARIANTS {
            let expected = if variant == Variant::Outline {
                theme.input
            } else {
                crate::theme::Color::TRANSPARENT
            };
            assert_eq!(variant.border(&theme), expected, "{}", variant.name());
        }
        assert_eq!(super::BORDER_WIDTH, px(1.0));
    }

    /// The `[button&,a&]:` group cannot fire on the element the app renders, and
    /// the port only lets it fire when the caller says the `render` prop made
    /// this a button or an anchor.
    #[test]
    fn hover_moves_nothing_on_a_span() {
        let theme = Theme::DARK;
        let mut badge = Badge::fixture();
        let resting = badge
            .variant
            .background(&theme, badge.state, badge.interactive);

        badge.state = BadgeState {
            hovered: true,
            ..BadgeState::default()
        };
        assert_eq!(
            badge.variant.background(&theme, badge.state, false),
            resting,
            "a <span> has no hover rule",
        );
        assert_ne!(
            badge.variant.background(&theme, badge.state, true),
            resting,
            "and a <button> does",
        );
    }

    /// The vocabularies are `cva`'s own keys, and the live/dead split is data
    /// rather than an opinion — counted over every `<Badge` in `web/src/`.
    #[test]
    fn the_vocabularies_are_closed_and_name_which_arms_are_live() {
        assert_eq!(ALL_SIZES.len(), 3);
        assert_eq!(ALL_VARIANTS.len(), 8);
        assert_eq!(ALL_CALL_SITES.len(), 3);

        let dead_sizes: Vec<&str> = ALL_SIZES
            .into_iter()
            .filter(|size| !size.live())
            .map(Size::name)
            .collect();
        assert_eq!(dead_sizes, ["lg"]);

        let live_variants: Vec<&str> = ALL_VARIANTS
            .into_iter()
            .filter(|variant| variant.live())
            .map(Variant::name)
            .collect();
        assert_eq!(live_variants, ["outline", "secondary", "warning"]);

        assert!(CallSite::None.live() && CallSite::Agent.live());
        assert!(
            !CallSite::Outdated.live(),
            "use-review-annotations.tsx never passes isOutdated"
        );
    }

    /// `empty` is a real picture on this component: with no label the box falls
    /// to `min-w-*`, which is the same `--spacing` step as the height — so an
    /// empty badge is a circle rather than a sliver.
    #[test]
    fn an_empty_badge_falls_to_its_min_width() {
        let mut badge = Badge::fixture();
        badge.label = None;

        assert_eq!(
            badge.size.min_width(badge.breakpoint),
            badge.size.height(badge.breakpoint),
        );
    }
}
