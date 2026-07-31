//! `switch` — **the first surface in this port whose `selected` state is
//! unambiguously real**, and the first where `--flags selected` earns its place
//! rather than being substituted or declared vacuous.
//!
//! The native half of `web/src/components/ui/switch.tsx`, which is two elements:
//! a `<button role="switch" data-slot="switch">` track and a
//! `<span data-slot="switch-thumb">` inside it. Every value below came out of the
//! app's own `index.css` through its own Tailwind 4.3.0 — see
//! `native/mapping/switch.md`.
//!
//! # Two recorded fields separate on from off, and both are compared
//!
//! Measured on the live pair (`/tmp/p3-ref-switch.json` against
//! `/tmp/p3-ref-switch-selected.json`), which is two *simultaneous* live
//! switches rather than one toggled between captures:
//!
//! | field | off | on |
//! |---|---|---|
//! | `switch.bg` | `#ffffff14` (`bg-input`) | `#516a36ff` (`bg-primary`) |
//! | `switch-thumb.bounds.x` | `1` | `13` |
//!
//! Nothing else moves — `bounds.w`/`h`, `radius`, `border` and `visible` are
//! identical on both anchors in both states. So this cell **can** fail, in two
//! independent ways, on two different anchors.
//!
//! # v1.9, and why the slide is reported by *both* sides here
//!
//! `ANCHORS.md` v1.9 is asymmetric: `WebKit`'s `getBoundingClientRect()` returns
//! the transformed box and gpui rotates at *paint*, so an animated transform
//! moves the reference's record and never the native one. The thumb slides, so
//! that is the first thing to rule out — and it resolves the other way.
//!
//! **The property in flight is `translate`, not `transform`.** Measured live:
//! the checked thumb reports `transform: "none"` and `translate: "12px"`,
//! because Tailwind 4 compiles `translate-x-*` to the standalone `translate`
//! property. `WebKit` folds it into `getBoundingClientRect()` exactly as it folds
//! a `transform` — the checked thumb reports `x: 13` against the resting `1`.
//!
//! The native side reaches the same box by **layout** ([`Switch::thumb_offset`]
//! as a left margin) rather than by a paint-time transform, so the driver's
//! prepaint bounds carry it. That is not a knob that manufactures agreement: it
//! supplies no number the reference produced. `translate-x-[calc(var(--thumb-size)-4px)]`
//! is an *input* — a class — and the port resolves it through its own
//! `--spacing`, exactly as P3.1 drew the line for `--class-radius`. The two
//! spellings are indistinguishable to the contract because the thumb is the
//! track's only child, so no sibling's layout depends on which one is used.
//!
//! The identity that makes this exact rather than lucky is asserted rather than
//! asserted-about: a flush-right thumb sits at `(2t − 2) − 2·1 − t = t − 4`, which
//! **is** the class's own `calc(var(--thumb-size) - 4px)`, at every thumb size.
//! See `the_checked_thumb_is_flush_against_the_tracks_inner_edge`.
//!
//! The transition is real — `[transition:translate_.15s,…]` — so a capture taken
//! inside 150ms of a toggle is v1.9's stated hole. Both live references were
//! taken from switches that had been in their state since mount, and the checked
//! one recorded `getComputedStyle(thumb).translate === "12px"` at the instant of
//! capture, so it is pinned rather than assumed.
//!
//! # `rounded-full` is `f32::MAX`, not gpui's `rounded_full()`
//!
//! P3.3's finding, met here for the second time and confirmed live: the track
//! reports `border-radius: 340282346638528859811704183484516925440px`, which is
//! **exactly `f32::MAX`** — `WebKit`'s resolution of `calc(infinity * 1px)`. gpui's
//! `rounded_full()` preset is `px(9999.)`, a 3.4e38 delta on a field compared at
//! ±0.5. [`TRACK_RADIUS`] is `px(f32::MAX)`.
//!
//! # `border` is **0** on both anchors
//!
//! `kbd`'s side of the trap rather than `button`'s: `switch.tsx` writes no
//! `border-*` on either element, so Tailwind's preflight `border: 0 solid`
//! stands. Measured live at `borderTopWidth: "0px"` on the track and on the
//! thumb. `border.w` is compared **exactly**, and the reference's
//! `border.color: "#ffffff0f"` on both is the junk v1.3 ruling 2 tells the differ
//! to ignore while `w == 0`.
//!
//! # The `sm:` trap fires forward here
//!
//! `[--thumb-size:--spacing(5)] sm:[--thumb-size:--spacing(4)]` — the primitive
//! authors both, and above 640px the `sm:` arm wins. A real window is 1714px, so
//! the live switch is the **16px** thumb in a `30 × 18` track, not the 20px thumb
//! in a `38 × 22` one its unprefixed class reads as.
//!
//! # The `size` prop is visually inert — measured, not inferred
//!
//! `switch.tsx` takes `size?: 'sm' | 'default'` and spends it on `data-size={size}`
//! and nothing else. There is no `data-size` in the class list, and a sweep of
//! **every** loaded stylesheet for a rule whose selector mentions `data-size`
//! returned **0**. **All 19 live call sites pass `size="sm"`** and it changes no
//! compared field, so it is not modelled here — see `native/mapping/switch.md`.

use gpui::{AnyElement, BoxShadow, Div, ParentElement as _, Pixels, Styled as _, div, px};

use super::anchor::{AnchorId, AnchorSink};
use super::git_status_row::Breakpoint;
use crate::theme::{Color, Theme};

/// The root anchor: the `<button role="switch" data-slot="switch">` track that
/// carries the background and the `rounded-full` corner.
///
/// It **has** to be the root: it is the outer of the two elements, and
/// `native/oracle/ANCHORS.md` §4 puts the thumb's bounds relative to it — which
/// is the whole point here, because the thumb's `x` is what `selected` moves.
pub const ID_SWITCH: &str = "switch";

/// The `<span data-slot="switch-thumb">` — the sliding knob.
pub const ID_THUMB: &str = "switch-thumb";

/// The anchors on this surface whose boxes size to their own text
/// (`native/oracle/ANCHORS.md` v1.5).
///
/// **None.** Neither element contains a text node at all, and both axes of both
/// boxes are authored: the track from `h-[calc(…)]`/`w-[calc(…)]`, the thumb
/// from `h-full` plus `aspect-square`. There is no max-content width anywhere on
/// this surface even in principle.
pub const CONTENT_SIZED: [&str; 0] = [];

/// The anchors on this surface whose **box height is their own line box**
/// (`native/oracle/ANCHORS.md` v1.6).
///
/// **None**, and unlike `input` this one is not even a near miss: a switch paints
/// no text, so neither side emits a `font` and v1.6's comparison has nothing on
/// either end. Declaring it would be the `git-row-badge` mistake with no line box
/// to be wrong about.
pub const LINE_SIZED: [&str; 0] = [];

/// `--spacing`, Tailwind's stock `0.25rem` at a 16px root.
///
/// Spelled once, as `dropdown_menu`, `input`, `resizable` and `tabs` spell it, so
/// a theme that moved the step would move every length below together.
/// `theme.css` does **not** redefine it.
const SPACING: f32 = 4.0;

/// `p-px` — the track's padding, on all four sides.
///
/// A literal CSS pixel rather than a `--spacing` multiple: `p-px` is Tailwind's
/// own one-pixel utility and does not scale with the step. It is what puts the
/// thumb at `x: 1, y: 1` inside the track, which the reference reports on both
/// cells.
pub const TRACK_PADDING: Pixels = px(1.0);

/// `rounded-full` on the track — **exactly `f32::MAX`**, not `px(9999.)`.
///
/// P3.3's finding, confirmed again on this surface's own live reference:
/// `WebKit` resolves `calc(infinity * 1px)` to `f32::MAX` and reports
/// `3.4028234663852886e+38`. gpui's `rounded_full()` preset would be off by
/// 3.4e38 on a field compared at ±0.5.
pub const TRACK_RADIUS: Pixels = px(f32::MAX);

/// `border` on both anchors — **zero, on every cell**.
///
/// `kbd`'s side of `native/MAPPING.md`'s border trap. `switch.tsx` writes no
/// `border-*` on either element, so preflight's `border: 0 solid` stands, and
/// `ANCHORS.md` v1.1 compares this field **exactly**.
pub const BORDER_WIDTH: Pixels = px(0.0);

/// `h-[calc(var(--thumb-size)+2px)]` — the two pixels the track is taller than
/// its thumb.
///
/// A literal `2px` in the class list, authored **separately** from `p-px`, and
/// kept a separate constant for `input`'s reason: they are two independent
/// declarations that happen to agree, and writing one of them twice would hide a
/// change to either. [`a_tracks_extra_height_is_its_own_two_padding_pixels`]
/// pins the agreement.
const TRACK_EXTRA_HEIGHT: f32 = 2.0;

/// `w-[calc(var(--thumb-size)*2-2px)]` — the track is two thumbs wide, less two
/// pixels.
const TRACK_WIDTH_DEFICIT: f32 = 2.0;

/// `data-checked:translate-x-[calc(var(--thumb-size)-4px)]` — how far the thumb
/// slides, in pixels below the thumb's own size.
///
/// Four, and it is not arbitrary: it is `2 × TRACK_PADDING` plus
/// [`TRACK_WIDTH_DEFICIT`], which is exactly the arithmetic that makes the
/// checked thumb sit flush against the track's inner right edge. See
/// [`Switch::thumb_offset`].
const THUMB_SLIDE_DEFICIT: f32 = 4.0;

/// `data-disabled:opacity-64`.
///
/// **Invisible to the differ**: `ANCHORS.md` v1.7's `visible` term fires only at
/// *zero*, and the contract has no opacity field. Painted for fidelity, exactly
/// as `button`'s and `input`'s identical `opacity-64` are.
pub const DISABLED_OPACITY: f32 = 0.64;

/// `focus-visible:ring-2` — **a box-shadow**, not a border.
///
/// The `ring-1` trap `native/MAPPING.md` records twice. A port that reached for
/// `.border_2()` here would report `border.w: 2` against the reference's `0`, on
/// the one field the contract compares exactly. Doubly dead: `document.hasFocus()`
/// is false and immovable on this machine, so no cell can reach it either.
pub const RING_WIDTH: Pixels = px(2.0);

/// `focus-visible:ring-offset-1` — the gap between the track and its ring.
///
/// Unlike `input`, this component **does** write a `ring-offset-*` beside its
/// `ring-offset-background`, so the offset layer is real rather than sitting at
/// its `0px` initial. Still a box-shadow, still `ANCHORS.md` §6.
pub const RING_OFFSET: Pixels = px(1.0);

/// `in-[…:active]:not-data-disabled:scale-x-110` — the thumb squashes wider
/// while the control is pressed.
///
/// **Unmodelled, and it could not be modelled usefully.** It is a `scale`, and
/// the contract has no field for one; on the reference it would move
/// `bounds.w` through `getBoundingClientRect()`, and on the native side gpui
/// applies scale at paint (v1.9's asymmetry, the direction that *does* bite).
/// `:active` is also a pointer state `ANCHORS.md` §6 puts out of reach of the
/// GPUI extractor entirely. Recorded as a number so a reader comparing this file
/// against the class list does not have to wonder.
pub const ACTIVE_SCALE_X: f32 = 1.10;

// **There is deliberately no `is_dark` helper on this component.** Every other
// ported surface carries one, because every other surface has a `dark:` variant
// to resolve. `switch.tsx` has **none** — the substring `dark:` appears zero
// times in it — so the theme reaches this component only through the token
// table, and a helper here would be a branch with nothing on the other side of
// it. Recorded rather than left as an absence, because "the port forgot the dark
// variant" and "there is no dark variant" look identical in a diff.

/// The `--thumb-size` custom property, which is the only length on this
/// component that is not derived from it.
///
/// `[--thumb-size:--spacing(5)] sm:[--thumb-size:--spacing(4)]` — authored by
/// the **primitive**, on both sides of the breakpoint, so unlike `label` there is
/// no call site that can beat it and unlike `badge` there is no unprefixed class
/// to lose to a `sm:` one. The live app is above 640px, so every live switch is
/// the 16px arm.
#[must_use]
pub const fn thumb_size(breakpoint: Breakpoint) -> Pixels {
    px(SPACING
        * match breakpoint {
            Breakpoint::Base => 5.0,
            Breakpoint::Sm => 4.0,
        })
}

/// One `Switch`: the track and its thumb.
#[derive(Clone, Copy, Debug, Default, PartialEq, Eq)]
pub struct Switch {
    /// Which side of `sm:` (640px) the **viewport** is on.
    pub breakpoint: Breakpoint,
    /// `checked` — §8.3's `selected`, and the state this whole surface exists to
    /// exercise.
    ///
    /// **A parameter rather than a gpui refinement**, per `ANCHORS.md` §6: the
    /// extractor reads each element's base `StyleRefinement` at prepaint, so a
    /// component that expressed this as `.toggle_state(…)` would report its
    /// resting appearance in every cell and converge while proving nothing.
    pub checked: bool,
    /// The `disabled` prop. Live — three call sites pass it — and invisible:
    /// `opacity-64` has no field and does not reach v1.7's zero.
    pub disabled: bool,
    /// `:focus-visible`. Real in the class list, unreachable in practice, and
    /// invisible either way — the ring is a box-shadow.
    pub focused: bool,
}

impl Switch {
    /// The live git-settings switch, as
    /// `web/src/features/settings/components/tabs/git-settings.tsx` renders it:
    /// `<Switch checked={…} size="sm" />`, at rest, above the `sm:` breakpoint.
    ///
    /// **This call site rather than one of the other five**, for `input`'s
    /// reason: it is the one the reference was captured from, and its root's
    /// subtree holds its own two anchors and nothing else, so `ANCHORS.md` v1.8
    /// is satisfied without a declaration.
    ///
    /// `checked: false` is the resting cell. Both states were live
    /// *simultaneously* in the Git tab — one switch on, one off — so neither
    /// reference had to be produced by driving the other.
    #[must_use]
    pub fn fixture() -> Self {
        Self {
            breakpoint: Breakpoint::Sm,
            checked: false,
            disabled: false,
            focused: false,
        }
    }

    /// `--thumb-size` at this cell's breakpoint. The thumb is a square of it.
    #[must_use]
    pub const fn thumb_extent(self) -> Pixels {
        thumb_size(self.breakpoint)
    }

    /// The track's height: `h-[calc(var(--thumb-size)+2px)]`.
    #[must_use]
    pub fn track_height(self) -> Pixels {
        self.thumb_extent() + px(TRACK_EXTRA_HEIGHT)
    }

    /// The track's width: `w-[calc(var(--thumb-size)*2-2px)]`.
    #[must_use]
    pub fn track_width(self) -> Pixels {
        self.thumb_extent() * 2.0 - px(TRACK_WIDTH_DEFICIT)
    }

    /// How far the thumb sits from the track's inner left edge.
    ///
    /// Zero when off; `calc(var(--thumb-size) - 4px)` when on — the class's own
    /// arithmetic, resolved through this port's own `--spacing` rather than
    /// copied from the reference's `12`.
    ///
    /// **Expressed as layout, where the original expresses it as `translate`.**
    /// See the module docs: `WebKit` folds `translate` into
    /// `getBoundingClientRect()` and the driver reads prepaint layout bounds, so
    /// this is the spelling that makes *both* sides report the move. The thumb is
    /// the track's only child, so nothing else's layout can tell the difference.
    #[must_use]
    pub fn thumb_offset(self) -> Pixels {
        if self.checked {
            self.thumb_extent() - px(THUMB_SLIDE_DEFICIT)
        } else {
            px(0.0)
        }
    }

    /// The thumb's radius: `rounded-(--thumb-size)`, the thumb's own size.
    ///
    /// Not `rounded-full`, and the difference is visible in the reference: the
    /// track reports `f32::MAX` and the thumb reports **16**. A port that gave
    /// both the same corner would be wrong on one of them.
    #[must_use]
    pub const fn thumb_radius(self) -> Pixels {
        self.thumb_extent()
    }

    /// The track's background — **the first of the two fields `selected` moves**.
    ///
    /// `data-checked:bg-primary` against `data-unchecked:bg-input`. Two different
    /// tokens rather than one token in two states, and neither carries a `dark:`
    /// variant: `switch.tsx` contains no `dark:` at all, so the theme reaches this
    /// only through the token table. Measured `#516a36ff` on and `#ffffff14` off.
    ///
    /// **And `--primary` is theme-invariant** — `Theme::LIGHT.primary` and
    /// `Theme::DARK.primary` are the identical `Hsla`, because the brand green is
    /// one colour in both tables. So `--theme` moves this field on the **off**
    /// cell and not on the on cell, and the checked switch stays theme-sensitive
    /// only through [`Switch::thumb_background`]. Found by a test that asserted
    /// the opposite and failed.
    #[must_use]
    pub fn track_background(&self, theme: &Theme) -> Color {
        if self.checked {
            theme.primary
        } else {
            theme.input
        }
    }

    /// The thumb's background: `bg-background`, in both states.
    ///
    /// Deliberately **not** a field `selected` moves — the reference reports
    /// `#1f1f1eff` on both cells — and asserted as such, so that a port which
    /// tinted the checked thumb would fail rather than look like an improvement.
    #[must_use]
    pub fn thumb_background(theme: &Theme) -> Color {
        theme.background
    }

    /// `focus-visible:ring-ring` — the halo's colour.
    ///
    /// **Invisible on every cell**: it is a box-shadow and `ANCHORS.md` §6 has no
    /// field for one. Kept correct so a reader comparing this file against the
    /// class list does not have to wonder.
    #[must_use]
    pub fn ring_color(theme: &Theme) -> Color {
        theme.ring
    }

    /// `focus-visible:ring-offset-background` — the offset layer's colour.
    #[must_use]
    pub fn ring_offset_color(theme: &Theme) -> Color {
        theme.background
    }

    /// Renders the track and the thumb, opting both into `anchors`.
    ///
    /// The root **is** the outermost element: `RowSurface` already draws the
    /// surface inside a `--width` box, and the track pins its own two axes rather
    /// than taking them from it. `inline-flex` blockifies to `flex` in the
    /// reference because every live track is a flex item, and gpui's `.flex()`
    /// reaches the same computed value without inline flow existing at all — the
    /// same reading `input` made of its control.
    #[must_use]
    pub fn render(&self, theme: &Theme, anchors: &dyn AnchorSink) -> AnyElement {
        let thumb = anchors.boxed(AnchorId::from(ID_THUMB), self.thumb(theme));
        anchors.root(ID_SWITCH.into(), self.track(theme).child(thumb))
    }

    /// The track's own box.
    fn track(self, theme: &Theme) -> Div {
        let mut element = div()
            .flex()
            .flex_shrink_0()
            .items_center()
            .w(self.track_width())
            .h(self.track_height())
            .p(TRACK_PADDING)
            .rounded(TRACK_RADIUS)
            .bg(self.track_background(theme));

        if self.focused {
            element = ring(element, theme);
        }
        if self.disabled {
            element = element.opacity(DISABLED_OPACITY);
        }
        element
    }

    /// The thumb's own box.
    ///
    /// `aspect-square h-full` becomes two explicit axes: the height is the
    /// track's content box — which `h-full` resolves to and which is
    /// `track_height - 2 × TRACK_PADDING`, i.e. the thumb size — and
    /// `aspect-square` makes the width the same. Written as
    /// [`Switch::thumb_extent`] twice rather than as a percentage plus an aspect
    /// ratio, because taffy resolving `h-full` against a padded parent is a
    /// second thing to be wrong about and the number is authored either way.
    fn thumb(self, theme: &Theme) -> Div {
        div()
            .flex_shrink_0()
            .ml(self.thumb_offset())
            .w(self.thumb_extent())
            .h(self.thumb_extent())
            .rounded(self.thumb_radius())
            .bg(Self::thumb_background(theme))
            // `shadow-sm/5`: `ANCHORS.md` §6 has no field for a shadow, and
            // gpui's preset is the same `0 1px 2px 0` at 5% the app compiles.
            // The literal lives inside gpui rather than in a component, which is
            // what keeps `check-invariants.sh` rule 4 satisfied.
            .shadow_sm()
    }
}

/// Adds `focus-visible:ring-2 ring-offset-1`'s two shadow layers in front of
/// whatever shadow `element` already carries.
///
/// A local copy of the one `dropdown_menu`, `resizable`, `tabs`, `button` and
/// `input` each carry, for their reason: the components are ported independently
/// and a shared helper would make one surface's diff reach into another's file.
///
/// There **are** two layers here, unlike `input`'s one. `switch.tsx` writes
/// `ring-offset-1` beside `ring-offset-background`, so `--tw-ring-offset-width`
/// is a real `1px` and the offset layer paints — the case `input`'s docs describe
/// as absent there.
fn ring(mut element: Div, theme: &Theme) -> Div {
    let offset = BoxShadow::new(px(0.0), px(0.0), Switch::ring_offset_color(theme).value())
        .spread_radius(RING_OFFSET);
    let halo = BoxShadow::new(px(0.0), px(0.0), Switch::ring_color(theme).value())
        .spread_radius(RING_OFFSET + RING_WIDTH);
    let shadows = element.style().box_shadow.get_or_insert_with(Vec::new);
    shadows.insert(0, halo);
    shadows.insert(0, offset);
    element
}

#[cfg(test)]
mod tests {
    use super::{
        ACTIVE_SCALE_X, BORDER_WIDTH, CONTENT_SIZED, DISABLED_OPACITY, ID_SWITCH, ID_THUMB,
        LINE_SIZED, RING_OFFSET, RING_WIDTH, Switch, TRACK_PADDING, TRACK_RADIUS, thumb_size,
    };
    use crate::components::Breakpoint;
    use crate::theme::Theme;
    use gpui::px;

    /// The fixture is the live git-settings switch, and the resting cell is the
    /// **off** one.
    #[test]
    fn the_fixture_is_the_live_git_settings_switch() {
        let switch = Switch::fixture();

        assert_eq!(switch.breakpoint, Breakpoint::Sm);
        assert!(!switch.checked);
        assert!(!switch.disabled);
        assert!(!switch.focused);

        // The live numbers, from `/tmp/p3-ref-switch.json`: a 30 × 18 track
        // holding a 16 × 16 thumb at (1, 1).
        assert_eq!(switch.track_width(), px(30.0));
        assert_eq!(switch.track_height(), px(18.0));
        assert_eq!(switch.thumb_extent(), px(16.0));
        assert_eq!(switch.thumb_offset(), px(0.0));
        assert_eq!(TRACK_PADDING, px(1.0));
    }

    /// **The `sm:` arm is the live one**, and the base arm is a genuinely
    /// different picture — the control that stops the breakpoint axis being
    /// decoration.
    #[test]
    fn the_thumb_size_follows_the_viewport_across_the_breakpoint() {
        const STEP: f32 = 4.0;

        assert_eq!(thumb_size(Breakpoint::Base), px(STEP * 5.0));
        assert_eq!(thumb_size(Breakpoint::Sm), px(STEP * 4.0));
        assert_ne!(thumb_size(Breakpoint::Base), thumb_size(Breakpoint::Sm));

        // Every length on the surface follows it, so the two breakpoints are two
        // different switches rather than one switch with a different knob.
        let base = Switch {
            breakpoint: Breakpoint::Base,
            ..Switch::fixture()
        };
        let sm = Switch::fixture();
        assert_eq!(base.track_width(), px(38.0));
        assert_eq!(base.track_height(), px(22.0));
        assert_eq!(sm.track_width(), px(30.0));
        assert_eq!(sm.track_height(), px(18.0));
        assert_ne!(base.thumb_radius(), sm.thumb_radius());
    }

    /// **The two fields `selected` moves, and the ones it must not.**
    ///
    /// This is the assertion the whole item turns on: `--flags selected` is real
    /// on this surface precisely because these two differ and nothing else does.
    #[test]
    fn selected_moves_the_track_background_and_the_thumbs_offset_and_nothing_else() {
        for theme in [Theme::LIGHT, Theme::DARK] {
            let off = Switch::fixture();
            let on = Switch {
                checked: true,
                ..Switch::fixture()
            };

            // 1. the track's background — two different tokens.
            assert_eq!(off.track_background(&theme), theme.input);
            assert_eq!(on.track_background(&theme), theme.primary);
            assert_ne!(off.track_background(&theme), on.track_background(&theme));

            // 2. the thumb's offset — 0 against `--thumb-size - 4px`.
            assert_eq!(off.thumb_offset(), px(0.0));
            assert_eq!(on.thumb_offset(), px(12.0));
            assert_ne!(off.thumb_offset(), on.thumb_offset());

            // And nothing else. Each of these is a compared field that the live
            // reference reports identically on both cells, so a port that moved
            // one would be wrong in a way the differ would catch.
            assert_eq!(off.track_width(), on.track_width());
            assert_eq!(off.track_height(), on.track_height());
            assert_eq!(off.thumb_extent(), on.thumb_extent());
            assert_eq!(off.thumb_radius(), on.thumb_radius());
            assert_eq!(
                Switch::thumb_background(&theme),
                Switch::thumb_background(&theme),
            );
            assert_eq!(off.thumb_radius(), px(16.0));
        }
    }

    /// **The checked thumb sits flush against the track's inner right edge** —
    /// which is what makes expressing the slide as layout exact rather than
    /// approximately right.
    ///
    /// `(2t − 2) − 2·1 − t = t − 4` for any `t`, so the class's own
    /// `calc(var(--thumb-size) - 4px)` **is** the flush-right position. Checked at
    /// both breakpoints, because a coincidence at one thumb size would not be one
    /// at the other.
    #[test]
    fn the_checked_thumb_is_flush_against_the_tracks_inner_edge() {
        for breakpoint in [Breakpoint::Base, Breakpoint::Sm] {
            let on = Switch {
                breakpoint,
                checked: true,
                ..Switch::fixture()
            };

            let content_width = on.track_width() - TRACK_PADDING * 2.0;
            let flush_right = content_width - on.thumb_extent();

            assert_eq!(
                on.thumb_offset(),
                flush_right,
                "{breakpoint:?}: the class's t-4 is the flush-right position",
            );
            // And it is not zero, or "flush right" and "flush left" would be the
            // same place and this would prove nothing.
            assert!(on.thumb_offset() > px(0.0), "{breakpoint:?}");
        }

        // The two breakpoints slide by different amounts, so the derivation is
        // doing work rather than agreeing with a constant.
        let base = Switch {
            breakpoint: Breakpoint::Base,
            checked: true,
            ..Switch::fixture()
        };
        let sm = Switch {
            checked: true,
            ..Switch::fixture()
        };
        assert_eq!(base.thumb_offset(), px(16.0));
        assert_eq!(sm.thumb_offset(), px(12.0));
        assert_ne!(base.thumb_offset(), sm.thumb_offset());
    }

    /// **A track's extra height is its own two padding pixels**, and the two are
    /// still separate declarations.
    ///
    /// `input`'s near-miss shape: `h-[calc(var(--thumb-size)+2px)]` and `p-px` are
    /// authored independently in the same class list, and their agreement is what
    /// makes `h-full` on the thumb resolve to `--thumb-size`. Asserted rather than
    /// collapsed into one constant, so a change to either is visible.
    #[test]
    fn a_tracks_extra_height_is_its_own_two_padding_pixels() {
        for breakpoint in [Breakpoint::Base, Breakpoint::Sm] {
            let switch = Switch {
                breakpoint,
                ..Switch::fixture()
            };
            assert_eq!(
                switch.track_height() - TRACK_PADDING * 2.0,
                switch.thumb_extent(),
                "{breakpoint:?}: h-full on the thumb is --thumb-size",
            );
        }
    }

    /// **`rounded-full` is `f32::MAX`, and the thumb's corner is not it** —
    /// P3.3's finding, and the check that this component did not apply it to both
    /// elements.
    #[test]
    fn the_track_is_f32_max_round_and_the_thumb_is_its_own_size() {
        assert_eq!(TRACK_RADIUS, px(f32::MAX));
        // The live reference reports `3.4028234663852886e+38`, which is `f32::MAX`
        // printed at f64 precision — an f32 literal cannot carry those digits, so
        // the equality above is the whole of that claim.
        //
        // What is worth asserting separately is the trap itself: gpui's
        // `rounded_full()` preset is `px(9999.)`, and a port that reached for it
        // would be 3.4e38 out on a field compared at ±0.5.
        assert_ne!(TRACK_RADIUS, px(9999.0), "gpui's rounded_full() preset");

        for breakpoint in [Breakpoint::Base, Breakpoint::Sm] {
            let switch = Switch {
                breakpoint,
                ..Switch::fixture()
            };
            assert_eq!(switch.thumb_radius(), switch.thumb_extent());
            assert_ne!(switch.thumb_radius(), TRACK_RADIUS);
        }
        assert_eq!(Switch::fixture().thumb_radius(), px(16.0));
    }

    /// **`border` is zero on both anchors** — `kbd`'s side of the trap, and a
    /// field compared exactly.
    #[test]
    fn neither_anchor_paints_a_border() {
        assert_eq!(BORDER_WIDTH, px(0.0));
        // The ring is a shadow at a real width, and it is *not* the border — the
        // `ring-1` trap, at two and three pixels.
        assert_ne!(RING_WIDTH, BORDER_WIDTH);
        assert_eq!(RING_WIDTH, px(2.0));
        assert_eq!(RING_OFFSET, px(1.0));
    }

    /// The two anchor ids are distinct and mirror the two `data-slot`s, and the
    /// root is the **outer** element.
    #[test]
    fn the_two_anchor_ids_are_the_two_data_slots() {
        assert_eq!(ID_SWITCH, "switch");
        assert_eq!(ID_THUMB, "switch-thumb");
        assert_ne!(ID_SWITCH, ID_THUMB);
        // Every bound is relative to the track, which is what lets the thumb's
        // `x` carry `selected`.
        assert!(ID_THUMB.starts_with(ID_SWITCH));
    }

    /// Neither declaration is made, and on this surface neither is close.
    #[test]
    fn the_surface_declares_neither_content_nor_line_sizing() {
        assert_eq!(CONTENT_SIZED, [] as [&str; 0]);
        assert_eq!(LINE_SIZED, [] as [&str; 0]);
    }

    /// **No `dark:` variant exists on this component**, so the theme reaches it
    /// only through the token table — and **`--primary` is the same colour in
    /// both tables**, which makes the checked track's background
    /// theme-*invariant*.
    ///
    /// Measured, not assumed: `Theme::LIGHT.primary` and `Theme::DARK.primary`
    /// are the identical `Hsla`. So `--theme` moves the **off** track and the
    /// thumb, and does **not** move the on track. A test that had asserted "both
    /// cells differ across themes" would have been wrong, and this one says which
    /// half is which so a reader is not left to infer it from a green run.
    #[test]
    fn the_theme_reaches_this_surface_only_through_the_token_table() {
        let off = Switch::fixture();
        let on = Switch {
            checked: true,
            ..Switch::fixture()
        };

        // The off track is `bg-input`, and the two tables differ — white/8% in
        // dark against black/10% in light.
        assert_ne!(
            off.track_background(&Theme::LIGHT),
            off.track_background(&Theme::DARK),
        );

        // The on track is `bg-primary`, and the brand green is one colour.
        assert_eq!(
            on.track_background(&Theme::LIGHT),
            on.track_background(&Theme::DARK),
        );
        assert_eq!(Theme::LIGHT.primary, Theme::DARK.primary);

        // The thumb is `bg-background`, which does move — so the checked cell is
        // still theme-sensitive overall, through the thumb rather than the track.
        assert_ne!(
            Switch::thumb_background(&Theme::LIGHT),
            Switch::thumb_background(&Theme::DARK),
        );
    }

    /// The two invisible states reach the component and neither can move a
    /// compared field.
    #[test]
    fn the_invisible_states_are_painted_and_cannot_fail() {
        let disabled = Switch {
            disabled: true,
            ..Switch::fixture()
        };
        let focused = Switch {
            focused: true,
            ..Switch::fixture()
        };

        // Neither touches geometry or the two colours the differ reads.
        for driven in [disabled, focused] {
            assert_eq!(driven.track_width(), Switch::fixture().track_width());
            assert_eq!(driven.track_height(), Switch::fixture().track_height());
            assert_eq!(driven.thumb_offset(), Switch::fixture().thumb_offset());
            assert_eq!(
                driven.track_background(&Theme::DARK),
                Switch::fixture().track_background(&Theme::DARK),
            );
        }

        // `opacity-64` does not reach v1.7's zero, so `visible` stays true on
        // both sides — the same reading `button` and `input` make.
        assert!((DISABLED_OPACITY - 0.64).abs() < f32::EPSILON);
        const { assert!(DISABLED_OPACITY > 0.0) };

        // The pressed squash is recorded and unmodelled: a scale, which the
        // contract has no field for and which the two engines report from
        // different stages of the pipeline.
        assert!((ACTIVE_SCALE_X - 1.10).abs() < f32::EPSILON);
    }
}
