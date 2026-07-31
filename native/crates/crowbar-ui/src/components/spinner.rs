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
//! # The port does not rotate, and that is deliberate
//!
//! [`Spinner::render`] paints a static box. Rotating it would put the **native**
//! side's bounds in flight too, and two snapshots taken at two unrelated
//! instants of the same 1s loop would disagree by up to 6.63px with nothing
//! wrong on either side. A snapshot is one instant (`ANCHORS.md` §6); the one
//! instant both sides can name is `t = 0`.
//!
//! It is a real visual gap in the same family as `skeleton`'s unswept sheen —
//! stated plainly rather than hidden — and unlike that one it is a gap the
//! oracle *can* see, which is why the rest instant is a rule here and a note
//! there.
//!
//! # The glyph is an empty box, and its colour is uncomparable
//!
//! The same call every component since `git_status_row` has made about icons:
//! the arc is an SVG a library draws, there is no native equivalent, and drawing
//! a substitute would put a shape on screen for the oracle to converge on.
//!
//! The consequence here is sharper than usual. The `<svg>` has **no text
//! nodes**, so the React extractor emits no `fg` for it — `oracleOwnText`
//! returns `""` and the whole text group is skipped. `stroke="currentColor"` is
//! the only colour the glyph has, and **no field in the contract records it**.
//! The reference confirms it: the anchor carries `bounds`, `bg`, `visible`,
//! `radius` and `border` and nothing else.

use gpui::{AnyElement, Div, Pixels, Styled as _, div, px};
use std::time::Duration;

use super::anchor::{AnchorId, AnchorSink};
use super::git_status_row::Breakpoint;

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
    /// Not a property of what the port paints — the port does not rotate — but
    /// of what a mis-timed capture of the original would record. See the module
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
    fn shell(self) -> Div {
        let extent = self.size();
        div().flex_shrink_0().w(extent).h(extent)
    }

    /// The element, with its one anchor.
    pub fn render(self, anchors: &dyn AnchorSink) -> AnyElement {
        anchors.boxed(AnchorId::from(ID_SPINNER), self.shell())
    }
}

#[cfg(test)]
mod tests {
    use super::{
        ALL_CALL_SITES, CallSite, Extent, ID_SPINNER, INTRINSIC_SIZE, MAX_ROTATED_EXTENT, PERIOD,
        SIZE_3, SIZE_4, SIZE_4_5, Spinner,
    };
    use crate::components::git_status_row::{BREAKPOINT_SM, Breakpoint};
    use gpui::px;

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
