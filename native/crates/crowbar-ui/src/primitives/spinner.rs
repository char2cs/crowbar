//! `spinner` — a rotating glyph, and **the first component `ANCHORS.md` v1.9
//! actually bites**.
//!
//! The native half of `web/src/components/ui/spinner.tsx`: lucide's
//! `Loader2Icon` with `animate-spin` and whatever `className` a call site
//! merges. Every value below came out of the app's own `tailwindcss` 4.3.0 with
//! the utility as a candidate — the method `native/MAPPING.md` fixes — and each
//! is confirmed against the captured reference. See `native/mapping/spinner.md`
//! and `/tmp/p3-ref-spinner.json`.
//!
//! # v1.9 **does** reach this component, and the excursion is 6.6px
//!
//! P3.6 checked `skeleton` the right way — *which* property animates against
//! *which* the contract reads — and concluded the capture was
//! timing-independent in every recorded field. The same check run here gives the
//! opposite answer, which is why it is worth doing per component rather than
//! once.
//!
//! `animate-spin` compiles to `animation: spin 1s linear infinite` over
//! `@keyframes spin { 100% { transform: rotate(360deg) } }`. The property in
//! flight is **`transform`** — and `transform` moves
//! `getBoundingClientRect()`, which is the exact call
//! `oracleRelativeBounds` feeds. Measured on the live element by stepping the
//! CSS animation's own timeline (`Element.getAnimations()`), against a layout
//! box that never moves off 16 × 16:
//!
//! ```text
//! t (ms)   rotation   bounds.w/h   bounds.x
//!      0        0°       16.000      936.500
//!     62.5     22.5°     20.905      934.047
//!    125       45°       22.627      933.186   ← the excursion
//!    187.5     67.5°     20.905      934.047
//!    250       90°       16.000      936.500
//!    500      180°       16.000      936.500
//! ```
//!
//! So `bounds.w`, `bounds.h`, `bounds.x` **and** `bounds.y` are all animated
//! recorded fields, by up to **6.63px** against §5's ±0.5px tolerance. Four
//! instants in every 1000ms are at rest and the other 996 are not: a capture
//! taken without care is not merely noisy, it is **wrong nearly always**, and
//! v1.9's warning — a snapshot cannot say when it was taken — is the whole
//! answer to a reader who sees four deltas on this anchor.
//!
//! The reference was therefore taken with the animation pinned at its origin
//! (`animation.pause(); animation.currentTime = 0`), which is `rotate(0deg)` and
//! so exactly the layout box. That is stronger than "captured at rest" by luck:
//! the instant is *chosen*, and the captured `transform` is recorded as
//! `matrix(1, 0, 0, 1, 0, 0)`.
//!
//! # The port **does** rotate, and the two questions separate cleanly
//!
//! An earlier draft of this module painted a static box and disclosed it as a
//! visual gap. Disclosing it was right; shipping it was not — §17's deliverable
//! is that a user cannot tell the two apps apart, and a spinner that does not
//! spin fails that at a glance, on the one component whose entire purpose is
//! motion. Two different things were being conflated:
//!
//! * **what the app does** — it must turn, or the port has shipped a broken
//!   spinner;
//! * **what the harness measures** — one instant, and both sides must name the
//!   same one.
//!
//! They separate for free, and for two independent reasons:
//!
//! 1. **`CROWBAR_ROW_SNAPSHOT` emits the *first frame* and quits**, and gpui's
//!    `AnimationElement` stamps `start = Instant::now()` on its first
//!    `request_layout`. So the native capture is at `delta ≈ 0` **by
//!    construction**, with no pinning code and no special case. Measured rather
//!    than assumed: `row_layout::spinner` asserts the first frame's delta is
//!    below 1e-3 of a turn.
//! 2. **gpui's rotation is a paint-time transform and does not touch layout.**
//!    `Svg::with_transformation`'s own doc says so, and `Window::paint_path`
//!    tessellates into the scene without going near taffy — so the driver, which
//!    records *layout* bounds at prepaint, reports the same 16 × 16 at **every**
//!    delta. The native side is immune to its own rotation where `WebKit` is not,
//!    because `getBoundingClientRect()` returns the *transformed* box.
//!
//! Point 2 is the asymmetry worth carrying forward: pinning the **reference** at
//! `currentTime = 0` is still necessary, and pinning the native side is not
//! merely unnecessary but impossible to get wrong. `row_layout::spinner` proves
//! it by stepping frames and asserting the recorded box never moves.
//!
//! # The glyph is lucide's own arc, and its colour is uncomparable
//!
//! This is the one place the port departs from the "icons are empty boxes" rule
//! `git_status_row` set, and the departure is narrow. That rule exists because a
//! call site *chooses* the icon, so drawing a substitute would put a shape on
//! screen for the oracle to converge on. **`Spinner`'s glyph is not a
//! choice** — `Loader2Icon` is hardcoded in `spinner.tsx`, it is the whole
//! component, and there is nothing for a call site to vary. So it is drawn, from
//! lucide's own path data rather than by eye: see [`GLYPH_SWEEP_DEGREES`].
//!
//! Nothing about it reaches the oracle. It is painted by
//! [`Window::paint_path`](gpui::Window::paint_path) on an **unanchored** child,
//! so the anchor's `bg`, `radius` and `border.w` are untouched — the same
//! standing `resizable`'s hit strip and `button`'s `::before` overlay have.
//!
//! The colour is sharper still. The `<svg>` has **no text nodes**, so the React
//! extractor emits no `fg` for it — `oracleOwnText` returns `""` and the whole
//! text group is skipped. `stroke="currentColor"` is the only colour the glyph
//! has, and **no field in the contract records it**; the port reads
//! `Window::text_style().color`, which is gpui's `currentColor`, and neither
//! side can be checked against the other. The reference confirms the shape of
//! the hole: the anchor carries `bounds`, `bg`, `visible`, `radius` and `border`
//! and nothing else.
//!
//! ## What is **not** verified: the pixels
//!
//! Everything above the paint call is tested — [`arc_path`] builds at every call
//! site's size and at eight instants round the turn, its bounds are the right
//! fraction of the box, a quarter turn moves its vertices and a whole turn
//! returns them, and `row_layout::spinner` proves frames are scheduled and the
//! recorded box does not move. The one link left is `Window::paint_path` itself,
//! and it could not be checked here. **Both routes are blocked, and each was
//! tried rather than assumed:**
//!
//! * `Window::render_to_image` exists under `test-support` but returns
//!   *"no `HeadlessRenderer` configured"* — `TestAppContext` passes no renderer
//!   factory, and the Metal one is behind `gpui_macos`'s own `cfg(test)`, which
//!   this workspace cannot reach and `native/vendor/**` may not be edited to
//!   expose;
//! * `screencapture` is denied Screen Recording for this process
//!   (*"could not create image from rect"*), so the running binary cannot be
//!   photographed either.
//!
//! Stated as a gap rather than glossed: the arc is drawn by one unconditional
//! call on a path proved to exist, in a colour the surface sets deliberately
//! (see `surfaces/spinner.rs`), and a human running `crowbar-app --surface
//! spinner` closes it in a second.
//!
//! ## One stated difference, flagged rather than worked around
//!
//! Under **reduced motion the two disagree**, and it is the app that is right.
//! `gpui`'s `AnimationElement` renders a repeating animation at `delta 0.0` and
//! schedules no frames when `App::reduce_motion` is set — a blanket policy. The
//! web app's own rule is not blanket: measured in the running app's CSSOM, it is
//!
//! ```text
//! :not(.animate-spin, [data-essential-motion], [data-essential-motion] *) { animation-duration: 0.01ms !important; … }
//! ```
//!
//! — `.animate-spin` is **exempted by name**, which is a deliberate product
//! decision that a loading indicator is essential motion. Honouring it on the
//! native side needs a hand-written animation element, because `reduce_motion`
//! is consulted inside gpui's and there is no hook. Recorded here and in
//! `native/mapping/spinner.md` rather than silently diverging.

use gpui::{
    Animation, AnimationExt as _, AnyElement, Div, IntoElement as _, ParentElement as _, Path,
    PathBuilder, Pixels, Point, SharedString, Styled as _, canvas, div, linear, point, px,
};
use std::time::Duration;

use crate::anchor::{AnchorId, AnchorSink};
use crate::surfaces::rows::git_status_row::Breakpoint;

/// The single anchor this surface carries.
pub const ID_SPINNER: &str = "spinner";

/// **Nothing.** The box is authored by a `size-*` utility at every call site,
/// and the bare primitive by lucide's own `width`/`height` attributes. Neither
/// is a text run's max-content width.
pub const CONTENT_SIZED: [&str; 0] = [];

/// **Nothing.** The glyph paints no text, so there is no line box for a height
/// to be derived from; `ANCHORS.md` v1.6 makes the declaration valid only on an
/// anchor that carries a `font`.
pub const LINE_SIZED: [&str; 0] = [];

/// lucide's own default `size`, which it writes as `width="24" height="24"`.
///
/// Live only where no `size-*` class and no `size` prop overrides it — which is
/// no call site in this app. Read off the live element's attributes rather than
/// off lucide's source.
pub const INTRINSIC_SIZE: Pixels = px(24.0);

/// `size-4` — `calc(var(--spacing) * 4)` at the stock `--spacing: 0.25rem`.
/// Measured live as `width: 16px`, and the reference's box.
pub const SIZE_4: Pixels = px(16.0);

/// `size-3` — 12px, `LoadingSpinner`'s `compact` shape.
pub const SIZE_3: Pixels = px(12.0);

/// `size-4.5` — 18px. The **unprefixed** half of the button's descendant rule.
pub const SIZE_4_5: Pixels = px(18.0);

/// One turn of `animate-spin`: Tailwind's stock `--animate-spin`, measured live
/// on the running app as `animation: 1s linear infinite spin`.
///
/// **Not a Crowbar token** — `theme.css` defines `--animate-skeleton` and five
/// others and does not touch this one, so there is nothing sealed to read it
/// from. It is carried because it is the length of the window in which a
/// capture of this surface is wrong: see the module docs.
pub const PERIOD: Duration = Duration::from_secs(1);

/// lucide `loader-circle`'s viewBox side, in its own units.
///
/// Every glyph constant below is a **ratio to this**, so the arc scales with
/// whatever box a call site pins rather than carrying a second set of numbers.
pub const GLYPH_VIEWBOX: f32 = 24.0;

/// The arc's radius, in [`GLYPH_VIEWBOX`] units — lucide's `a9 9`.
pub const GLYPH_RADIUS: f32 = 9.0;

/// The arc's stroke width, in [`GLYPH_VIEWBOX`] units — lucide's
/// `stroke-width="2"`.
pub const GLYPH_STROKE: f32 = 2.0;

/// How far round the circle the arc runs, in degrees.
///
/// **Derived from lucide's path data rather than judged by eye.**
/// `loader-circle` is `M21 12a9 9 0 1 1-6.219-8.56` in a 24×24 viewBox: the
/// centre is `(12, 12)`, so the start `(21, 12)` is `(9, 0)` from it — angle 0,
/// three o'clock — and the end `(14.781, 3.44)` is `(2.781, -8.56)`, whose
/// radius is 9.000 and whose angle in screen coordinates is **287.998°**. The
/// `large-arc-flag` is `1`, which selects the major arc, so the sweep is 288 and
/// not 72. Reproducing it: `9·cos 288° = 2.781`, `9·sin 288° = -8.560`.
pub const GLYPH_SWEEP_DEGREES: f32 = 288.0;

/// The `with_animation` element id the turn runs under.
///
/// A constant, exactly as `gpui_component::Spinner` uses `"circle"` for its own.
/// Two spinners under one identified ancestor therefore **share** an animation
/// clock and turn in phase, which is a nuisance at worst and never a panic; no
/// surface in this port renders two.
const TURN_ELEMENT_ID: &str = "spinner-turn";

/// The largest a rotating square's bounding box gets, as a multiple of its side.
///
/// A square rotated 45° has a bounding box `√2` times its side, which is where
/// the 22.627 in the module docs' table comes from. Written down because it is
/// the number that decides whether v1.9 reaches a component at all: multiplied
/// by the box and set against §5's ±0.5px, it says the capture instant is
/// load-bearing here where it was not on `skeleton`.
pub const MAX_ROTATED_EXTENT: f32 = std::f32::consts::SQRT_2;

/// The `className` bundle a call site merges over the primitive's own.
///
/// `spinner.tsx` has exactly two importers, and **only one of them is live**.
#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub enum CallSite {
    /// No className: lucide's own [`INTRINSIC_SIZE`]. No call site renders it —
    /// both importers merge something.
    None,
    /// `loading-spinner.tsx`'s `size-4`. **This is the captured cell.**
    LoadingSpinner,
    /// `loading-spinner.tsx`'s `size-3`, the `compact` shape.
    LoadingSpinnerCompact,
    /// `button.tsx`'s loading indicator, `pointer-events-none absolute`.
    ///
    /// **Dead**: `loading` is never passed to a `<Button>` anywhere in
    /// `web/src/` — the two greps that look like it are
    /// `disabled={… || loading}`. It is modelled anyway because it is the one
    /// place in this item where the `sm:` breakpoint moves a compared number:
    /// the className carries no `size-`, so the button's own
    /// `[&_svg:not([class*='size-'])]:size-4.5 sm:[&_svg…]:size-4` sizes it, at
    /// **18px below 640px and 16px at or above**.
    ///
    /// Its `absolute` is not modelled and needs no excuse: on this surface the
    /// spinner **is** the root anchor, so `ANCHORS.md` §4 puts it at the origin
    /// by construction and its position is not a compared field.
    ButtonLoadingIndicator,
}

/// Every modelled call site, for `--help` and the closed-vocabulary test.
pub const ALL_CALL_SITES: [CallSite; 4] = [
    CallSite::None,
    CallSite::LoadingSpinner,
    CallSite::LoadingSpinnerCompact,
    CallSite::ButtonLoadingIndicator,
];

impl CallSite {
    /// The word `--call-site` takes.
    #[must_use]
    pub fn name(self) -> &'static str {
        match self {
            Self::None => "none",
            Self::LoadingSpinner => "loading-spinner",
            Self::LoadingSpinnerCompact => "loading-spinner-compact",
            Self::ButtonLoadingIndicator => "button-loading-indicator",
        }
    }

    /// The box this call site pins at a given viewport.
    ///
    /// Only [`CallSite::ButtonLoadingIndicator`] reads the breakpoint; the rest
    /// carry an unprefixed `size-*` with no `sm:` counterpart anywhere, which
    /// was checked rather than assumed — see `native/mapping/spinner.md`.
    #[must_use]
    pub fn extent(self, breakpoint: Breakpoint) -> Pixels {
        match self {
            Self::None => INTRINSIC_SIZE,
            Self::LoadingSpinner => SIZE_4,
            Self::LoadingSpinnerCompact => SIZE_3,
            Self::ButtonLoadingIndicator => match breakpoint {
                Breakpoint::Base => SIZE_4_5,
                Breakpoint::Sm => SIZE_4,
            },
        }
    }

    /// Whether this call site's extent moves at 640px.
    #[must_use]
    pub fn follows_breakpoint(self) -> bool {
        matches!(self, Self::ButtonLoadingIndicator)
    }
}

/// What decides the glyph's box.
///
/// A vocabulary rather than a `Pixels`, for `kbd::Cap`'s reason: the two cases
/// are different pictures, and one of them has no box at all.
#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub enum Extent {
    /// The `className` a call site merges.
    Class(CallSite),
    /// **Zero area.** `Spinner` takes `React.ComponentProps<typeof Loader2Icon>`
    /// and forwards them, so `size={0}` writes `width="0" height="0"` with no
    /// `size-*` class to override them; the box paints nothing and the extractor
    /// reports `visible: false`. §8.3's `empty` is that cell — `skeleton`'s
    /// precedent, reached from the props rather than from the absence of a
    /// class. **No live call site passes it.**
    Empty,
}

/// One `<Spinner>`.
#[derive(Clone, Copy, Debug, PartialEq)]
pub struct Spinner {
    /// What sizes the glyph.
    pub extent: Extent,
    /// Which side of `sm:` the viewport is on. Read only by
    /// [`CallSite::ButtonLoadingIndicator`].
    pub breakpoint: Breakpoint,
}

impl Spinner {
    /// The captured cell: the `LoadingSpinner` inside a freshly-mounted
    /// `ReviewDiffTab`, at a 1714px viewport — `16 × 16`, `bg #00000000`,
    /// `radius 0`, `border.w 0`, `visible: true`, and **no text group at all**.
    #[must_use]
    pub fn fixture() -> Self {
        Self {
            extent: Extent::Class(CallSite::LoadingSpinner),
            breakpoint: Breakpoint::Sm,
        }
    }

    /// The box this cell paints, in logical px.
    #[must_use]
    pub fn size(self) -> Pixels {
        match self.extent {
            Extent::Class(call_site) => call_site.extent(self.breakpoint),
            Extent::Empty => px(0.0),
        }
    }

    /// How far the *reference's* `bounds` can travel from this box while the
    /// animation runs, in logical px.
    ///
    /// **Not a property of what the driver records.** gpui rotates at paint
    /// time, so the native `bounds` are the layout box at every delta; this is
    /// what a mis-timed capture of the *original* would record, because `WebKit`'s
    /// `getBoundingClientRect()` returns the transformed box. See the module
    /// docs; on the captured 16px cell it is 6.63.
    #[must_use]
    pub fn rotation_excursion(self) -> Pixels {
        self.size() * (MAX_ROTATED_EXTENT - 1.0)
    }

    /// The glyph's box.
    ///
    /// No background, no radius, no border: preflight's `border: 0 solid` stands
    /// — `spinner.tsx` carries no `border` class, which is `kbd`'s side of the
    /// trap rather than `badge`'s — and an `<svg>` has no background of its own.
    /// The reference agrees: `bg #00000000`, `radius 0`, `border.w 0`.
    ///
    /// **This is the anchored box, so nothing painted may land on it.** The arc
    /// goes on an unanchored child; see [`Spinner::turning_arc`].
    fn shell(self) -> Div {
        let extent = self.size();
        div().flex_shrink_0().w(extent).h(extent)
    }

    /// The turn, as gpui sees it: one [`PERIOD`], repeating, **linear**.
    ///
    /// Public so the layout harness can drive this exact configuration and read
    /// the deltas back — the first-frame instant the whole capture rests on is a
    /// property of these three settings, and a test that built its own animation
    /// would be measuring something else. `linear` and not
    /// `gpui_component::Spinner`'s `ease_in_out`: the CSS says
    /// `animation: spin 1s linear infinite`, and an eased turn is a visibly
    /// different motion.
    #[must_use]
    pub fn turn() -> Animation {
        Animation::new(PERIOD).repeat().with_easing(linear)
    }

    /// lucide's arc, turning once per [`PERIOD`], on an **unanchored** child.
    ///
    /// The child fills the anchored box and carries no id, so the extractor
    /// never sees it and the anchor's `bg`/`radius`/`border` stay the
    /// reference's. `Window::paint_path` tessellates into the scene and does not
    /// touch taffy, so the recorded *layout* bounds are the same at every delta
    /// — the property the module docs' point 2 turns on, and the one
    /// `row_layout::spinner` steps frames to prove.
    ///
    /// The colour is `Window::text_style().color` — gpui's `currentColor`, and
    /// the exact counterpart of the `stroke="currentColor"` the SVG carries.
    /// Read at paint time rather than passed in, so a host's `text-*` token
    /// reaches it the way the DOM's does.
    fn turning_arc(self) -> AnyElement {
        let extent = self.size();
        div()
            .size_full()
            .with_animation(
                SharedString::new_static(TURN_ELEMENT_ID),
                Self::turn(),
                move |element, delta| element.child(arc(extent, delta)),
            )
            .into_any_element()
    }

    /// The element, with its one anchor.
    pub fn render(self, anchors: &dyn AnchorSink) -> AnyElement {
        let shell = self.shell();
        // A zero-area box has nothing to draw in, and a radius-0 arc is a
        // degenerate tessellation rather than an invisible one.
        if self.size() <= px(0.0) {
            return anchors.boxed(AnchorId::from(ID_SPINNER), shell);
        }
        anchors.boxed(AnchorId::from(ID_SPINNER), shell.child(self.turning_arc()))
    }
}

/// One frame of the arc, as an element.
fn arc(extent: Pixels, turn: f32) -> impl gpui::IntoElement {
    canvas(
        |_, _, _| (),
        move |bounds, (), window, _| {
            if let Some(path) = arc_path(extent, bounds.center(), turn) {
                window.paint_path(path, window.text_style().color);
            }
        },
    )
}

/// lucide's geometry, rotated by `turn` of a full circle and centred on `centre`.
///
/// Built centred on the origin so the rotation is about the arc's own centre,
/// then translated onto the box — `PathBuilder::rotate` rotates about the origin
/// and `translate` chains after it, which is the order the two calls appear in.
///
/// `None` only where lyon refuses to tessellate. Split out from [`arc`] so that
/// **a failure to build is a test failure rather than an invisible spinner**:
/// swallowing it at the paint site is right (nothing can be done in `paint`) and
/// would otherwise be indistinguishable from the static box this component used
/// to draw.
fn arc_path(extent: Pixels, centre: Point<Pixels>, turn: f32) -> Option<Path<Pixels>> {
    let scale = f32::from(extent) / GLYPH_VIEWBOX;
    let radius = GLYPH_RADIUS * scale;
    let sweep = GLYPH_SWEEP_DEGREES.to_radians();

    let mut builder = PathBuilder::stroke(px(GLYPH_STROKE * scale));
    builder.move_to(point(px(radius), px(0.0)));
    builder.arc_to(
        point(px(radius), px(radius)),
        px(0.0),
        // `large-arc-flag`, `sweep-flag` — lucide's `0 1 1`.
        true,
        true,
        point(px(radius * sweep.cos()), px(radius * sweep.sin())),
    );
    builder.rotate(turn * 360.0);
    builder.translate(centre);
    builder.build().ok()
}

#[cfg(test)]
mod tests {
    use super::{
        ALL_CALL_SITES, CallSite, Extent, GLYPH_RADIUS, GLYPH_STROKE, GLYPH_VIEWBOX, ID_SPINNER,
        INTRINSIC_SIZE, MAX_ROTATED_EXTENT, PERIOD, SIZE_3, SIZE_4, SIZE_4_5, Spinner,
    };
    use crate::surfaces::rows::git_status_row::{BREAKPOINT_SM, Breakpoint};
    use gpui::{point, px};

    /// Neither declaration is made, and each for its own reason — see the
    /// constants.
    #[test]
    fn a_spinner_declares_neither_content_nor_line_sizing() {
        assert!(super::CONTENT_SIZED.is_empty());
        assert!(super::LINE_SIZED.is_empty());
    }

    /// **The v1.9 finding, as an assertion rather than a paragraph.** The
    /// rotation moves `bounds` by far more than §5's ±0.5px, which is what makes
    /// the capture instant load-bearing on this component where it was not on
    /// `skeleton`.
    ///
    /// The number is the measured one: the live 16px glyph's bounding box reads
    /// **22.627** at 45°, and 22.627 − 16 = 6.627.
    #[test]
    fn the_rotation_moves_the_bounds_far_outside_the_tolerance() {
        let fixture = Spinner::fixture();
        assert_eq!(fixture.size(), SIZE_4);

        let excursion = f32::from(fixture.rotation_excursion());
        assert!(
            (excursion - 6.627).abs() < 0.01,
            "16 × (√2 − 1) is the measured 22.627 − 16; got {excursion}",
        );
        // §5's bounds tolerance. The point of the assertion is the *ratio*: a
        // thirteen-fold overrun is not something a wider tolerance could absorb.
        assert!(excursion > 0.5 * 13.0, "got {excursion}");

        // And the excursion scales with the box, so no call site escapes it.
        for call_site in ALL_CALL_SITES {
            let spinner = Spinner {
                extent: Extent::Class(call_site),
                ..Spinner::fixture()
            };
            assert!(
                f32::from(spinner.rotation_excursion()) > 0.5,
                "{}",
                call_site.name(),
            );
        }
    }

    /// A whole turn is one second, so the four instants at which the bounding
    /// box equals the layout box are 0, 250, 500 and 750ms — which is why a
    /// capture taken without pinning the timeline is wrong almost always.
    #[test]
    fn the_turn_is_one_second_and_only_the_quarter_turns_are_at_rest() {
        assert_eq!(PERIOD.as_millis(), 1000);
        // The extent is `|cos θ| + |sin θ|` times the side; it is 1 exactly at
        // the multiples of 90° and strictly greater everywhere between.
        for (degrees, at_rest) in [(0.0_f32, true), (45.0, false), (90.0, true), (91.0, false)] {
            let theta = degrees.to_radians();
            let extent = theta.cos().abs() + theta.sin().abs();
            assert_eq!((extent - 1.0).abs() < 1e-5, at_rest, "{degrees}°");
            assert!(extent <= MAX_ROTATED_EXTENT + 1e-5, "{degrees}°");
        }
    }

    /// The live sizes, and the one call site whose number moves at 640px.
    ///
    /// The control matters as much as the claim: three of the four carry an
    /// unprefixed `size-*` with no `sm:` counterpart, so a port that read the
    /// breakpoint everywhere would be wrong three times over.
    #[test]
    fn only_the_button_indicator_follows_the_breakpoint() {
        assert_eq!(CallSite::LoadingSpinner.extent(Breakpoint::Sm), SIZE_4);
        assert_eq!(
            CallSite::LoadingSpinnerCompact.extent(Breakpoint::Sm),
            SIZE_3
        );
        assert_eq!(CallSite::None.extent(Breakpoint::Sm), INTRINSIC_SIZE);

        for call_site in ALL_CALL_SITES {
            let base = call_site.extent(Breakpoint::Base);
            let small = call_site.extent(Breakpoint::Sm);
            assert_eq!(
                base != small,
                call_site.follows_breakpoint(),
                "{call_site:?}"
            );
        }

        assert_eq!(
            CallSite::ButtonLoadingIndicator.extent(Breakpoint::of(BREAKPOINT_SM - 1.0)),
            SIZE_4_5,
        );
        assert_eq!(
            CallSite::ButtonLoadingIndicator.extent(Breakpoint::of(BREAKPOINT_SM)),
            SIZE_4,
        );
    }

    /// §8.3's `empty`: a zero-area box, whatever the viewport says.
    #[test]
    fn the_empty_extent_has_no_box_at_all() {
        for breakpoint in [Breakpoint::Base, Breakpoint::Sm] {
            let spinner = Spinner {
                extent: Extent::Empty,
                breakpoint,
            };
            assert_eq!(spinner.size(), px(0.0));
            assert_eq!(spinner.rotation_excursion(), px(0.0));
        }
    }

    /// The fixture is the captured cell, and the anchor is the one the reference
    /// names.
    #[test]
    fn the_fixture_is_the_captured_review_tab_spinner() {
        let fixture = Spinner::fixture();
        assert_eq!(fixture.extent, Extent::Class(CallSite::LoadingSpinner));
        assert_eq!(fixture.breakpoint, Breakpoint::Sm);
        assert_eq!(fixture.size(), px(16.0));
        assert_eq!(ID_SPINNER, "spinner");
    }

    /// **The arc is really drawn**, at every call site's size and all the way
    /// round the turn.
    ///
    /// `arc` swallows a tessellation failure, because there is nothing a `paint`
    /// callback can do about one — and a swallowed failure looks exactly like
    /// the static box this component used to paint, which is the defect the
    /// coordinator sent this change back for. So the build is asserted here.
    ///
    /// The bounds are checked too, and they are the control: a path that built
    /// but collapsed to a point would pass an `is_some()` on its own.
    #[test]
    fn the_arc_builds_at_every_size_and_every_instant() {
        let centre = point(px(50.0), px(50.0));
        for call_site in ALL_CALL_SITES {
            let extent = call_site.extent(Breakpoint::Sm);
            for step in 0..8_u8 {
                let turn = f32::from(step) / 8.0;
                let path = super::arc_path(extent, centre, turn)
                    .unwrap_or_else(|| panic!("{} at {turn}", call_site.name()));
                assert!(!path.vertices.is_empty(), "{}", call_site.name());

                // lucide's arc is r = 9/24 of the box, stroked 2/24 wide, so its
                // nominal extent is 2r + stroke = 20/24 of the box on each axis.
                //
                // The range rather than an equality, on both sides and for two
                // different reasons: a 288° arc is missing a 72° notch, so one
                // axis can fall short of the full diameter at some instants;
                // and lyon's stroke tessellation overshoots the nominal box by
                // its join tolerance — measured at **20.013 against 20** on the
                // 24px cell, so 5% is generous and still rejects a path built at
                // the wrong scale, which is what this bound is for.
                let full = f32::from(extent) * (2.0 * GLYPH_RADIUS + GLYPH_STROKE) / GLYPH_VIEWBOX;
                let width = f32::from(path.bounds.size.width);
                let height = f32::from(path.bounds.size.height);
                assert!(
                    width > full * 0.5 && width <= full * 1.05,
                    "{} at {turn}: width {width} against {full}",
                    call_site.name(),
                );
                assert!(
                    height > full * 0.5 && height <= full * 1.05,
                    "{} at {turn}: height {height} against {full}",
                    call_site.name(),
                );
            }
        }
    }

    /// The turn is a **whole** circle, so the arc at delta 0 and the arc at
    /// delta 1 are the same picture — which is what makes a repeating animation
    /// seamless, and what makes "the first frame" and "the reference's
    /// `currentTime = 0`" the same instant.
    #[test]
    fn a_whole_turn_returns_the_arc_to_where_it_started() {
        let centre = point(px(8.0), px(8.0));
        let extent = Spinner::fixture().size();
        let start = super::arc_path(extent, centre, 0.0).expect("builds");
        let full = super::arc_path(extent, centre, 1.0).expect("builds");
        let quarter = super::arc_path(extent, centre, 0.25).expect("builds");

        assert_eq!(start.vertices.len(), full.vertices.len());
        for (a, b) in start.vertices.iter().zip(full.vertices.iter()) {
            assert!((f32::from(a.xy_position.x - b.xy_position.x)).abs() < 0.01);
            assert!((f32::from(a.xy_position.y - b.xy_position.y)).abs() < 0.01);
        }
        // The control: a quarter turn is a *different* picture, so the
        // assertion above is about the period rather than about a static path.
        assert!(
            start
                .vertices
                .iter()
                .zip(quarter.vertices.iter())
                .any(|(a, b)| (f32::from(a.xy_position.x - b.xy_position.x)).abs() > 0.5),
            "a quarter turn must move the arc",
        );
    }

    /// The vocabulary is closed and its words are unique.
    #[test]
    fn the_call_site_vocabulary_is_closed() {
        let mut names: Vec<_> = ALL_CALL_SITES.iter().map(|c| c.name()).collect();
        names.sort_unstable();
        assert_eq!(
            names,
            [
                "button-loading-indicator",
                "loading-spinner",
                "loading-spinner-compact",
                "none",
            ],
        );
    }
}
