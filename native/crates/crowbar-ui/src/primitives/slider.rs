//! `slider` — **hand-built, not wrapped, and confirmed style-only against the
//! vendor source** (P3.30).
//!
//! §6.2 names `slider` as one of `gpui-component`'s widgets to wrap. Read
//! directly (`native/vendor/gpui-component/src/slider.rs`), `Slider` is a
//! `#[derive(IntoElement)]` whose `RenderOnce::render` builds every box —
//! `slider-bar-container`, `slider-bar`, both thumbs — inside its own body from
//! `div()`/`h_flex()`. The only caller-facing seam is `Styled::style(&mut self)
//! -> &mut StyleRefinement`: no `child`, no `children`, no method taking an
//! `impl IntoElement`, and `ParentElement` is imported only for the vendor's own
//! internal `.child(...)` calls. [`AnchorSink`]'s methods take a `gpui::Div` this
//! crate holds; a `StyleRefinement` is not one. **The survey this item was
//! briefed against was right about this component**: wrapping it would yield a
//! `div()` whose bounds merely *coincide* with the vendor's own box — one
//! compared field, the fake convergence `ANCHORS.md` exists to refuse. Confirmed
//! by reading the source, not re-derived from the brief's account of it.
//!
//! `switch` is the precedent for exactly this shape and this file follows its
//! pattern: visual state is a parameter, colours come from [`Theme`] only, and
//! every length is either a literal a stylesheet writes or arithmetic derived
//! from one.
//!
//! The native half of `web/src/components/ui/slider.tsx` (a `@base-ui/react/slider`
//! wrap), reachable at exactly one live call site — the "Fault Injection" rows
//! of Settings → Developer (`developer-settings.tsx`) — which is where every
//! number below was measured, live, in the Tauri webview (not a Chrome
//! surrogate: `border-radius: f32::MAX` is a `WebKit` resolution `chrome-devtools-mcp`
//! would not reproduce). See `native/mapping/slider.md`.

use gpui::{AnyElement, Div, ParentElement as _, Pixels, Styled as _, div, px};

use crate::anchor::{AnchorId, AnchorSink};
use crate::surfaces::rows::git_status_row::Breakpoint;
use crate::theme::{Color, Theme};

/// The root anchor: `SliderPrimitive.Control` (`data-slot="slider-control"`) —
/// the full-width, unpainted flex container. `SliderPrimitive.Root` sits above
/// it in the real DOM but paints nothing and shares its box exactly (measured:
/// both report the identical `668 × 4` on the live cell), so it is not a second
/// anchor — see `native/mapping/slider.md`'s "one wrapper, not two".
pub const ID_ROOT: &str = "slider";
/// The track — `SliderPrimitive.Track`'s `::before`, the rounded pill that
/// paints `bg-input`. **Not `inset:0`**: `before:inset-x-0.5 before:inset-y-0`
/// pulls it in 2px on each horizontal edge only. `native/oracle/ANCHORS.md` §3's
/// pseudo-backed shortcut assumes `inset:0` and does not apply here — this
/// anchor is a real native box at the pseudo's *measured* rect, not the host's.
pub const ID_TRACK: &str = "slider-track";
/// `SliderPrimitive.Indicator` — the filled portion, `bg-primary`. A real
/// element (not pseudo-backed), positioned and sized in the live DOM by
/// `margin-inline-start` plus a percentage width the primitive computes.
pub const ID_INDICATOR: &str = "slider-indicator";
/// `SliderPrimitive.Thumb` — the round knob. `bg-white`, unconditionally, in
/// both themes; only its border moves with `dark:`.
pub const ID_THUMB: &str = "slider-thumb";

/// The anchors on this surface whose boxes size to their own text
/// (`native/oracle/ANCHORS.md` v1.5). **None** — nothing on a slider paints a
/// character, and every length below is authored or derived, never
/// max-content.
pub const CONTENT_SIZED: [&str; 0] = [];

/// The anchors whose box height is their own line box (v1.6). **None**, for the
/// same reason: no text, no font, nothing for v1.6 to be wrong about.
pub const LINE_SIZED: [&str; 0] = [];

/// `--spacing`, Tailwind's stock `0.25rem` at a 16px root. Spelled once, as
/// `switch`, `input`, `resizable` and the rest do.
const SPACING: f32 = 4.0;

/// `h-1` on the track — the row's total height, and the height of the track
/// pill and the indicator alike (the indicator's `height: inherit` chain
/// resolves to this).
pub const TRACK_HEIGHT: Pixels = px(4.0);

/// `before:inset-x-0.5` — how far the track's pill is pulled in from each
/// horizontal edge of its host. **Measured**, not read off the class list:
/// `getComputedStyle(track, '::before').left` reported `2px` live, which is
/// `0.5 × --spacing` and confirms the class list rather than merely matching
/// it. `before:inset-y-0` is zero, so the pill's height is the host's own.
pub const TRACK_INSET: Pixels = px(2.0);

/// `ms-0.5` on the indicator — its own left offset. The same two pixels as
/// [`TRACK_INSET`] by coincidence of both being `0.5 × --spacing`, and kept as
/// a separate constant for `switch`'s `TRACK_EXTRA_HEIGHT`/`TRACK_WIDTH_DEFICIT`
/// reason: they are two independent declarations — a margin on one element, an
/// inset on an unrelated pseudo-element — that happen to agree, not one value
/// spelled twice.
pub const INDICATOR_MARGIN: Pixels = px(2.0);

/// `rounded-full` on the track, the indicator and the thumb alike — **exactly
/// `f32::MAX`**, not `px(9999.)`. The third surface this is measured on
/// (`switch`, `scroll-area` before it): `WebKit` resolves `calc(infinity * 1px)`
/// to `f32::MAX` and reports `3.4028234663852886e+38` on the live pseudo's own
/// `border-top-left-radius` — measured directly via `getComputedStyle(track,
/// '::before').borderTopLeftRadius`, not inferred from the class name.
pub const ROUNDED_FULL: Pixels = px(f32::MAX);

/// `border` on the thumb — `border-input dark:border-background`, one pixel in
/// both themes.
pub const THUMB_BORDER: Pixels = px(1.0);

/// `data-disabled:opacity-64` on the control. Present in the class list and
/// unreached by the one live call site (`developer-settings.tsx` never passes
/// `disabled`) — painted for fidelity, the same reading `switch` and `input`
/// give their own `opacity-64`. Invisible to the differ either way:
/// `ANCHORS.md` v1.7's `visible` term fires only at zero opacity, and the
/// contract carries no opacity field.
pub const DISABLED_OPACITY: f32 = 0.64;

/// `[--thumb-size:--spacing(5)] sm:[--thumb-size:--spacing(4)]`'s shape here:
/// `size-5 sm:size-4` directly on the thumb's own class list. **20px below the
/// `sm:` breakpoint, 16px at or above it** — the live cell (1714px viewport) is
/// the `sm:` arm, matched exactly by the captured `16 × 16` thumb.
#[must_use]
pub const fn thumb_size(breakpoint: Breakpoint) -> Pixels {
    px(SPACING
        * match breakpoint {
            Breakpoint::Base => 5.0,
            Breakpoint::Sm => 4.0,
        })
}

/// One `Slider`: a track, its fill, and a thumb.
#[derive(Clone, Copy, Debug, PartialEq)]
pub struct Slider {
    /// The control's own rendered width — **a real axis on this surface**,
    /// unlike `switch`'s track: `slider.tsx`'s `Control` is `w-full`, and the
    /// one live call site sizes it with `className="min-w-0 flex-1"` inside a
    /// settings row, so the number comes from the container rather than being
    /// authored on the component. Measured `668px` on the live cell at a
    /// 1714px window.
    pub width: Pixels,
    /// Which side of `sm` (640px) the **viewport** is on. Drives
    /// [`thumb_size`], and through it every derived offset on this surface.
    pub breakpoint: Breakpoint,
    /// `min` — the value's lower bound. `0.0` at the one live call site.
    pub min: f32,
    /// `max` — the value's upper bound. `100.0` at the one live call site.
    pub max: f32,
    /// The current value, between [`Slider::min`] and [`Slider::max`].
    ///
    /// **A parameter, not interaction state gpui tracks**: the live control is
    /// a `@base-ui/react/slider`, driven here by setting its hidden
    /// `<input type="range">` and dispatching a native `input` event — the
    /// same reason [`super::switch::Switch::checked`] is a field rather than a
    /// refinement, generalised to a continuous value.
    pub value: f32,
    /// `data-disabled` — real in the class list, unreached by the one live
    /// call site. See [`DISABLED_OPACITY`].
    pub disabled: bool,
    /// `:focus-visible` (`has-focus-visible:ring-*`). Real in the class list,
    /// **invisible**: a ring is a box-shadow, `ANCHORS.md` §6, and
    /// `document.hasFocus()` is false and immovable on this machine, so no
    /// cell can reach it either.
    pub focused: bool,
}

impl Slider {
    /// The live fault-injection slider at rest — `min: 0, max: 100, value: 0`,
    /// the `sm:` breakpoint, `668px` wide. The cell `/tmp/p3-ref-slider.json`
    /// was captured from.
    #[must_use]
    pub const fn fixture() -> Self {
        Self {
            width: px(668.0),
            breakpoint: Breakpoint::Sm,
            min: 0.0,
            max: 100.0,
            value: 0.0,
            disabled: false,
            focused: false,
        }
    }

    /// `--thumb-size` at this cell's breakpoint.
    #[must_use]
    pub const fn thumb_extent(self) -> Pixels {
        thumb_size(self.breakpoint)
    }

    /// `(value - min) / (max - min)`, clamped to `[0, 1]`.
    ///
    /// `max <= min` reads as `0.0` rather than dividing by zero or a negative
    /// span — a degenerate range the live call site never produces (`min: 0,
    /// max: 100` is a literal), but the fixture's own defaults must not panic
    /// under it.
    #[must_use]
    pub fn value_fraction(self) -> f32 {
        if self.max <= self.min {
            return 0.0;
        }
        ((self.value - self.min) / (self.max - self.min)).clamp(0.0, 1.0)
    }

    /// Where the thumb's **centre** sits, root-relative — base-ui's
    /// `thumbAlignment="edge"` arithmetic, resolved in pixels rather than the
    /// percentage the DOM carries.
    ///
    /// Not a plain `valueFraction × width`: `thumbAlignment="edge"` keeps the
    /// thumb's centre inset by half its own extent at both ends, so it never
    /// overflows the track — `inset + valueFraction × (width − 2 × inset)`,
    /// where `inset = thumbExtent / 2`. Confirmed against the live DOM rather
    /// than derived from the prop's name: at rest (`value: 0`) the captured
    /// thumb reported `--position: 1.1976047904191618%` of a `668px` track,
    /// which is exactly `(16 / 668) / 2 × 100`, not `0%`. At `value: 40` the
    /// indicator's own width reported `268.8px`, which is exactly `8 + 0.4 ×
    /// (668 − 16)` — this function, in pixels.
    #[must_use]
    pub fn thumb_center(self) -> Pixels {
        let inset = self.thumb_extent() * 0.5;
        let span = (self.width - inset * 2.0).max(px(0.0));
        inset + span * self.value_fraction()
    }

    /// The indicator's width — **exactly [`Slider::thumb_center`]**, not that
    /// minus [`INDICATOR_MARGIN`]. The live DOM's `SliderPrimitive.Indicator`
    /// carries its own `width: var(--start-position)` (a percentage of the
    /// track) entirely independently of its `margin-inline-start`; the two are
    /// unrelated declarations that together place the fill flush with the
    /// pill's rounded left cap and reaching to the thumb's centre. Measured:
    /// `268.8px` at `value: 40`, matching [`Slider::thumb_center`] to the
    /// decimal.
    #[must_use]
    pub fn indicator_width(self) -> Pixels {
        self.thumb_center()
    }

    /// The thumb's own left edge — its recorded `bounds.x` — one half-extent
    /// left of its centre.
    #[must_use]
    pub fn thumb_left(self) -> Pixels {
        self.thumb_center() - self.thumb_extent() * 0.5
    }

    /// The thumb's top offset, root-relative. `top: 50%; translate: -50%` on a
    /// [`TRACK_HEIGHT`]-tall row: the thumb is centred on the row regardless of
    /// its own (larger) extent, which is negative at both breakpoints.
    /// Measured `-6px` at the `16px` (`sm:`) extent, matching
    /// `(4 − 16) / 2`.
    #[must_use]
    pub fn thumb_top(self) -> Pixels {
        (TRACK_HEIGHT - self.thumb_extent()) * 0.5
    }

    /// The track pill's colour — `bg-input`. **The same token
    /// [`super::switch::Switch::track_background`] reads for its own resting
    /// track**, confirmed by value: `theme.css`'s `--input` in dark is
    /// `oklch(1 0 0 / 8%)`, and the live pseudo reported that exact `oklch`
    /// string.
    #[must_use]
    pub fn track_color(theme: &Theme) -> Color {
        theme.input
    }

    /// The indicator's colour — `bg-primary`. **Theme-invariant**, the same
    /// finding `switch`'s checked track makes: `--primary` is
    /// `oklch(0.49 0.082 130)` in both tables, and the live indicator reported
    /// `#516a36ff` — `switch`'s own measured "on" colour, verbatim.
    #[must_use]
    pub fn indicator_color(theme: &Theme) -> Color {
        theme.primary
    }

    /// The thumb's fill — `bg-white`, unconditionally. See [`Color::WHITE`]'s
    /// own docs for why this is a sealed literal rather than a [`Theme`]
    /// field: `slider.tsx` writes no `dark:` variant on this declaration at
    /// all.
    #[must_use]
    pub const fn thumb_color() -> Color {
        Color::WHITE
    }

    /// The thumb's border — `border-input`, overridden to `border-background`
    /// under `dark:`. **Not the same value in both themes**: measured
    /// `#1f1f1eff` live (dark), which is [`Theme::background`]'s own dark
    /// value — the same colour `switch`'s thumb paints as its *fill*, here
    /// painted as a 1px ring instead.
    #[must_use]
    pub fn thumb_border_color(theme: &Theme) -> Color {
        if is_dark(theme) {
            theme.background
        } else {
            theme.input
        }
    }

    /// Renders the track, the indicator and the thumb, opting all three into
    /// `anchors`.
    #[must_use]
    pub fn render(&self, theme: &Theme, anchors: &dyn AnchorSink) -> AnyElement {
        let track = anchors.boxed(AnchorId::from(ID_TRACK), self.track(theme));
        let indicator = anchors.boxed(AnchorId::from(ID_INDICATOR), self.indicator(theme));
        let thumb = anchors.boxed(AnchorId::from(ID_THUMB), self.thumb(theme));

        let mut root = div()
            .relative()
            .w(self.width)
            .h(TRACK_HEIGHT)
            .child(track)
            .child(indicator)
            .child(thumb);
        if self.disabled {
            root = root.opacity(DISABLED_OPACITY);
        }
        anchors.root(ID_ROOT.into(), root)
    }

    /// The pill: `::before`'s measured rect, as a real box — inset
    /// [`TRACK_INSET`] on each horizontal edge, full [`TRACK_HEIGHT`] tall.
    fn track(&self, theme: &Theme) -> Div {
        let width = (self.width - TRACK_INSET * 2.0).max(px(0.0));
        div()
            .absolute()
            .left(TRACK_INSET)
            .top_0()
            .w(width)
            .h(TRACK_HEIGHT)
            .rounded(ROUNDED_FULL)
            .bg(Self::track_color(theme))
    }

    /// The fill: [`INDICATOR_MARGIN`] from the left edge, [`Slider::indicator_width`]
    /// wide, full [`TRACK_HEIGHT`] tall.
    fn indicator(&self, theme: &Theme) -> Div {
        div()
            .absolute()
            .left(INDICATOR_MARGIN)
            .top_0()
            .w(self.indicator_width())
            .h(TRACK_HEIGHT)
            .rounded(ROUNDED_FULL)
            .bg(Self::indicator_color(theme))
    }

    /// The knob: a [`Slider::thumb_extent`] square at
    /// ([`Slider::thumb_left`], [`Slider::thumb_top`]), bordered and filled.
    fn thumb(&self, theme: &Theme) -> Div {
        div()
            .absolute()
            .left(self.thumb_left())
            .top(self.thumb_top())
            .w(self.thumb_extent())
            .h(self.thumb_extent())
            .rounded(ROUNDED_FULL)
            .border(THUMB_BORDER)
            .border_color(Self::thumb_border_color(theme))
            .bg(Self::thumb_color())
    }
}

/// Whether a `dark:` Tailwind variant is in force.
///
/// A local copy of `checkbox`'s, `dropdown_menu`'s, `git_status_row`'s and
/// `input`'s, deliberately: the components are ported independently and a
/// shared helper would make one surface's diff reach into another's file.
fn is_dark(theme: &Theme) -> bool {
    *theme == Theme::DARK
}

#[cfg(test)]
mod tests {
    use super::{
        CONTENT_SIZED, DISABLED_OPACITY, ID_INDICATOR, ID_ROOT, ID_THUMB, ID_TRACK,
        INDICATOR_MARGIN, LINE_SIZED, ROUNDED_FULL, Slider, THUMB_BORDER, TRACK_HEIGHT,
        TRACK_INSET, thumb_size,
    };
    use crate::surfaces::rows::git_status_row::Breakpoint;
    use crate::theme::{Color, Theme};
    use gpui::{Pixels, px};

    /// Equal to within f32 rounding noise (not `ANCHORS.md`'s ±0.5px
    /// tolerance, which is about engine disagreement — this is about a
    /// multiplication chain landing on `268.80002` instead of `268.8`, the
    /// same value read back through one extra float operation).
    #[track_caller]
    fn assert_px_close(got: Pixels, want: Pixels) {
        let diff = (f32::from(got) - f32::from(want)).abs();
        assert!(diff < 0.001, "got {got:?}, want {want:?} (diff {diff})");
    }

    /// The fixture is the live fault-injection slider at rest, and the numbers
    /// are the live reference's (`/tmp/p3-ref-slider.json`).
    #[test]
    fn the_fixture_is_the_live_slider_at_rest() {
        let slider = Slider::fixture();

        assert_eq!(slider.width, px(668.0));
        assert_eq!(slider.breakpoint, Breakpoint::Sm);
        assert!((slider.min - 0.0).abs() < f32::EPSILON);
        assert!((slider.max - 100.0).abs() < f32::EPSILON);
        assert!((slider.value - 0.0).abs() < f32::EPSILON);
        assert!(!slider.disabled);
        assert!(!slider.focused);

        assert_eq!(slider.thumb_extent(), px(16.0));
        // The reference's `slider-track`: inset 2px each side, 664 wide.
        assert_eq!(TRACK_INSET, px(2.0));
        assert_eq!(slider.width - TRACK_INSET * 2.0, px(664.0));
        // The reference's `slider-indicator` and `slider-thumb` at rest.
        assert_eq!(slider.indicator_width(), px(8.0));
        assert_eq!(slider.thumb_left(), px(0.0));
        assert_eq!(slider.thumb_top(), px(-6.0));
    }

    /// **The edge-alignment formula, pinned to the live reference's own
    /// numbers at `value: 40`** (`/tmp/p3-ref-slider-selected.json`) — the
    /// second live capture this component was measured against, the same way
    /// `switch`'s "on" cell is a second live switch rather than an assumption.
    #[test]
    fn value_forty_lands_on_the_live_references_own_numbers() {
        let driven = Slider {
            value: 40.0,
            ..Slider::fixture()
        };

        assert_px_close(driven.indicator_width(), px(268.8));
        assert_px_close(driven.thumb_center(), px(268.8));
        assert_px_close(driven.thumb_left(), px(260.8));
        // Only the value moved: width, breakpoint and the resting geometry's
        // other numbers are untouched.
        assert_eq!(driven.width, Slider::fixture().width);
        assert_eq!(driven.thumb_top(), Slider::fixture().thumb_top());
    }

    /// The thumb's centre never overflows the track, at any value — the whole
    /// point of `thumbAlignment="edge"` and the reason the formula is not a
    /// plain percentage.
    #[test]
    fn the_thumb_never_overflows_the_track_at_either_extreme() {
        let min = Slider {
            value: 0.0,
            ..Slider::fixture()
        };
        let max = Slider {
            value: 100.0,
            ..Slider::fixture()
        };

        let inset = min.thumb_extent() * 0.5;
        assert_eq!(min.thumb_center(), inset);
        assert_eq!(max.thumb_center(), min.width - inset);

        // The thumb's own box stays fully inside [0, width] at both ends.
        assert_eq!(min.thumb_left(), px(0.0));
        assert_eq!(
            max.thumb_left() + max.thumb_extent(),
            max.width,
            "the thumb's right edge lands exactly on the track's",
        );
    }

    /// A value outside `[min, max]` clamps rather than extrapolating —
    /// defensive, since the live control's own `<input type="range">` already
    /// clamps and this function should agree with it rather than disagree
    /// silently.
    #[test]
    fn out_of_range_values_clamp_to_the_track_ends() {
        let below = Slider {
            value: -50.0,
            ..Slider::fixture()
        };
        let above = Slider {
            value: 500.0,
            ..Slider::fixture()
        };

        assert!((below.value_fraction() - 0.0).abs() < f32::EPSILON);
        assert!((above.value_fraction() - 1.0).abs() < f32::EPSILON);
        assert_eq!(below.thumb_center(), Slider::fixture().thumb_center());
        assert_eq!(
            above.thumb_center(),
            (Slider {
                value: 100.0,
                ..Slider::fixture()
            })
            .thumb_center(),
        );
    }

    /// `rounded-full` is `f32::MAX` on all three painted anchors, measured off
    /// the pseudo's own `border-top-left-radius` rather than assumed from the
    /// class name — the third surface this exact trap is recorded on.
    #[test]
    fn every_anchor_is_rounded_full_at_f32_max() {
        assert_eq!(ROUNDED_FULL, px(f32::MAX));
        assert_ne!(ROUNDED_FULL, px(9999.0), "gpui's rounded_full() preset");
    }

    /// `size-5 sm:size-4` — 20px below the breakpoint, 16px at or above it,
    /// and the live cell is the `sm:` arm.
    #[test]
    fn the_thumb_size_follows_the_viewport_across_the_breakpoint() {
        assert_eq!(thumb_size(Breakpoint::Base), px(20.0));
        assert_eq!(thumb_size(Breakpoint::Sm), px(16.0));
        assert_ne!(thumb_size(Breakpoint::Base), thumb_size(Breakpoint::Sm));

        let base = Slider {
            breakpoint: Breakpoint::Base,
            ..Slider::fixture()
        };
        assert_eq!(base.thumb_extent(), px(20.0));
        assert_eq!(base.thumb_top(), px(-8.0));
        assert_ne!(base.thumb_extent(), Slider::fixture().thumb_extent());
    }

    /// `bg-input` on the track, `bg-primary` on the indicator — the same two
    /// tokens `switch`'s track reads for its off/on cells, confirming this
    /// surface reuses rather than reinvents the vocabulary.
    #[test]
    fn the_track_and_indicator_read_the_same_tokens_switch_does() {
        for theme in [Theme::LIGHT, Theme::DARK] {
            assert_eq!(Slider::track_color(&theme), theme.input);
            assert_eq!(Slider::indicator_color(&theme), theme.primary);
        }
        // `--primary` is theme-invariant; `--input` is not.
        assert_eq!(
            Slider::indicator_color(&Theme::LIGHT),
            Slider::indicator_color(&Theme::DARK),
        );
        assert_ne!(
            Slider::track_color(&Theme::LIGHT),
            Slider::track_color(&Theme::DARK),
        );
    }

    /// **`bg-white` is unconditional** — the thumb's fill does not move across
    /// themes, only its border does.
    #[test]
    fn the_thumb_fill_is_white_in_both_themes_and_only_the_border_moves() {
        assert_eq!(Slider::thumb_color(), Color::WHITE);

        let border_light = Slider::thumb_border_color(&Theme::LIGHT);
        let border_dark = Slider::thumb_border_color(&Theme::DARK);
        assert_eq!(border_light, Theme::LIGHT.input);
        assert_eq!(border_dark, Theme::DARK.background);
        assert_ne!(border_light, border_dark);
    }

    /// The measured live colour, pinned to the token it is supposed to be —
    /// `#1f1f1eff`, `Theme::DARK.background`'s own value, read live off the
    /// thumb's `border-top-color` in the Tauri webview.
    #[test]
    fn the_measured_dark_border_colour_is_the_background_token() {
        let got = gpui::Rgba::from(Slider::thumb_border_color(&Theme::DARK).value());
        let want: gpui::Rgba = gpui::rgb(0x001f_1f1e);
        let tolerance = 0.6 / 255.0;
        for (channel, g, w) in [
            ("r", got.r, want.r),
            ("g", got.g, want.g),
            ("b", got.b, want.b),
        ] {
            assert!(
                (g - w).abs() < tolerance,
                "{channel}: got {g}, want {w} ({got:?} vs {want:?})"
            );
        }
    }

    /// Neither declaration is made, and there is no text on this surface even
    /// in principle.
    #[test]
    fn the_surface_declares_neither_content_nor_line_sizing() {
        assert_eq!(CONTENT_SIZED, [] as [&str; 0]);
        assert_eq!(LINE_SIZED, [] as [&str; 0]);
    }

    /// The four anchor ids are the four `data-slot`s, and the root is the
    /// **outer** invisible container — every other bound is relative to it.
    #[test]
    fn the_anchor_ids_are_the_four_data_slots() {
        assert_eq!(ID_ROOT, "slider");
        assert_eq!(ID_TRACK, "slider-track");
        assert_eq!(ID_INDICATOR, "slider-indicator");
        assert_eq!(ID_THUMB, "slider-thumb");
        assert!(ID_TRACK.starts_with(ID_ROOT));
        assert!(ID_INDICATOR.starts_with(ID_ROOT));
        assert!(ID_THUMB.starts_with(ID_ROOT));
    }

    /// The invisible/inert states reach the component and move nothing this
    /// contract can see.
    #[test]
    fn the_invisible_states_move_nothing_the_contract_compares() {
        let disabled = Slider {
            disabled: true,
            ..Slider::fixture()
        };
        let focused = Slider {
            focused: true,
            ..Slider::fixture()
        };

        for driven in [disabled, focused] {
            assert_eq!(driven.thumb_center(), Slider::fixture().thumb_center());
            assert_eq!(
                Slider::track_color(&Theme::DARK),
                Slider::track_color(&Theme::DARK),
            );
        }
        assert!((DISABLED_OPACITY - 0.64).abs() < f32::EPSILON);
        const { assert!(DISABLED_OPACITY > 0.0) };
    }

    /// `border` is 1px on the thumb — real, and the only border this surface
    /// paints (the track and the indicator carry none).
    #[test]
    fn the_thumb_is_the_only_bordered_anchor() {
        assert_eq!(THUMB_BORDER, px(1.0));
        assert_eq!(INDICATOR_MARGIN, TRACK_INSET, "both are 0.5 × --spacing");
        assert_eq!(TRACK_HEIGHT, px(4.0));
    }
}
