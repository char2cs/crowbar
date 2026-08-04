//! `flicker-spinner` — a flip-dot spinner, and the one of the three where v1.9
//! is ruled out by a **measurement on the anchored element itself**.
//!
//! The native half of `web/src/components/ui/flicker-spinner.tsx`: a
//! `size-4` span that random-picks one of 31 build-time SVGs and inlines its
//! markup. Every value below came out of the app's own `tailwindcss` 4.3.0 with
//! the utility as a candidate — the method `native/MAPPING.md` fixes — and each
//! is confirmed against the captured reference. See
//! `native/mapping/flicker-spinner.md` and `/tmp/p3-ref-flicker-spinner.json`.
//!
//! # v1.9 does **not** reach this component, and the check is a measurement
//!
//! The component is unmistakably animated — that is the whole point of it — so
//! v1.9's "a snapshot has no way to say *when* it was taken" is the first thing
//! to rule out. Two independent facts rule it out here:
//!
//! 1. **The animation is `fill-opacity`, on `<circle>` elements**, declared by
//!    SMIL `<animate calcMode="discrete">` inside the inlined SVG. Counted over
//!    the whole asset directory: **775 `<animate>` elements across 31 files, and
//!    all 775 carry `attributeName="fill-opacity"`** — there is no second
//!    animated property to have missed. No field in `ANCHORS.md` §3 records it:
//!    `fg` and `bg` read `color` and `background-color`, and `visible` reads
//!    `opacity`, which is a different property on a different element.
//! 2. **The animation is not on the anchored element at all.** The dots are
//!    unanchored descendants, and the span itself carries none:
//!    `Element.getAnimations()` on the live anchor returned **`[]`**, with
//!    `transform: none`. That is the sharpest possible contrast with
//!    [`crate::primitives::spinner`], where the same call returned the `spin`
//!    animation and stepping its timeline moved `bounds` by 6.63px.
//!
//! So a capture here is timing-independent in every recorded field, and would be
//! whether it were taken at rest or mid-flicker — the stronger statement P3.6
//! made about `skeleton`, arrived at the same way and confirmed on the element
//! rather than inferred from the stylesheet.
//!
//! # The dots are unmodelled, and are invisible in both directions
//!
//! The port paints the box; it does not draw 25 flip-dots or flicker them. Like
//! `skeleton`'s unswept sheen this is a real visual gap, and like it the
//! contract records neither the dots nor their opacity, so closing it would be
//! unverifiable by the oracle either way.
//!
//! Two further properties go with them, both stated rather than modelled:
//!
//! * **Which spinner is drawn is random per instance** — `SPINNERS[Math.floor(
//!   Math.random() * SPINNERS.length)]` over `import.meta.glob('./spinners/*.svg')`,
//!   31 files. A component whose output is not a function of its props would be
//!   a problem for any oracle; it is not a problem for *this* one, because the
//!   choice only ever changes unanchored markup.
//! * **The colour is `currentColor` and nothing here sets it.** The span paints
//!   no text nodes, so the React extractor emits no `fg` — `oracleOwnText`
//!   returns `""` and the whole text group is skipped. `text-foreground` on the
//!   pane's call site is therefore **uncomparable**, which is exactly the
//!   property the primitive's own comment is protecting.

use gpui::{AnyElement, Div, Pixels, Styled as _, div, px};
use std::time::Duration;

use crate::anchor::{AnchorId, AnchorSink};

/// The single anchor this surface carries.
pub const ID_FLICKER_SPINNER: &str = "flicker-spinner";

/// **Nothing.** The box is `size-4` on the primitive and a `size-*` at every
/// call site; no text run decides it.
pub const CONTENT_SIZED: [&str; 0] = [];

/// **Nothing.** The span paints no text, so there is no line box for a height to
/// be derived from.
pub const LINE_SIZED: [&str; 0] = [];

/// `size-4` — the primitive's own, `calc(var(--spacing) * 4)` = 16px.
pub const SIZE_4: Pixels = px(16.0);

/// `size-6` — 24px, `agent-chat-pane.tsx`'s. **The captured cell.**
pub const SIZE_6: Pixels = px(24.0);

/// `size-3.5` — 14px, `workspace-branch-icon.tsx`'s.
pub const SIZE_3_5: Pixels = px(14.0);

/// The shortest and longest flip-dot loop in `spinners/*.svg`.
///
/// Counted by reading every `dur=` in the directory: **twelve distinct values
/// from 0.36s to 1.62s** across 775 `<animate>` elements, all 775 of which
/// animate `fill-opacity` and nothing else.
///
/// **Not a Crowbar token** — the numbers are authored in the SVG assets, not in
/// `theme.css`, so there is nothing sealed to read them from. They are carried
/// because of what the *spread* says: there is no single settling time a capture
/// of this component could wait for, so the timing-independence argument has to
/// come from **which property animates** rather than from how long it takes.
/// Which is the argument, and it is the one `ANCHORS.md` v1.9 asks for.
pub const FLICKER_PERIOD_RANGE: (Duration, Duration) =
    (Duration::from_millis(360), Duration::from_millis(1620));

/// How many flip-dot SVGs `import.meta.glob('./spinners/*.svg')` finds, counted
/// in `web/src/components/ui/spinners/`.
///
/// The instance picks one at random and keeps it for its lifetime. Recorded
/// because "the rendered markup is not a function of the props" is worth knowing
/// about a component under a parity gate — see the module docs for why it is
/// nonetheless harmless here.
pub const SPINNER_VARIANTS: usize = 31;

/// The `className` bundle a call site merges over the primitive's own.
///
/// Three live call sites, and each one names its own `size-*`. tailwind-merge
/// keeps the last of a group, and **no `sm:` variant exists on either side** —
/// checked rather than assumed, which is what P3.3's trap costs when it is not.
#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub enum CallSite {
    /// No className: the primitive's own `size-4`.
    None,
    /// `agent-chat-glyph.tsx`'s `size-4` — a call site **restating** the
    /// primitive's own size. Modelled separately because it is a distinct call
    /// site, and asserted to land on the same number.
    ChatGlyph,
    /// `agent-chat-pane.tsx`'s `size-6 text-foreground`, shown while a spawn the
    /// pane asked for is in flight. **This is the captured cell.**
    ChatPane,
    /// `workspace-branch-icon.tsx`'s `size-3.5`, inside `WorkspaceAgentSpinner`'s
    /// own `size-4` box.
    WorkspaceIcon,
}

/// Every modelled call site, for `--help` and the closed-vocabulary test.
pub const ALL_CALL_SITES: [CallSite; 4] = [
    CallSite::None,
    CallSite::ChatGlyph,
    CallSite::ChatPane,
    CallSite::WorkspaceIcon,
];

impl CallSite {
    /// The word `--call-site` takes.
    #[must_use]
    pub fn name(self) -> &'static str {
        match self {
            Self::None => "none",
            Self::ChatGlyph => "chat-glyph",
            Self::ChatPane => "chat-pane",
            Self::WorkspaceIcon => "workspace-icon",
        }
    }

    /// The box this call site pins.
    #[must_use]
    pub fn extent(self) -> Pixels {
        match self {
            Self::None | Self::ChatGlyph => SIZE_4,
            Self::ChatPane => SIZE_6,
            Self::WorkspaceIcon => SIZE_3_5,
        }
    }
}

/// One `<FlickerSpinner>`.
#[derive(Clone, Copy, Debug, PartialEq)]
pub struct FlickerSpinner {
    /// The `className` bundle a call site merges over the primitive's own.
    pub call_site: CallSite,
    /// §8.3's `empty`: no box at all.
    ///
    /// The primitive hardcodes `size-4`, so the only way to reach a zero box is
    /// a call site merging a `size-0` over it — a real branch of the one prop
    /// this component takes, and **no live call site renders it**. The picture
    /// is `skeleton`'s: zero area, and the extractor reports `visible: false`.
    ///
    /// The other reading of "nothing in it" — `SPINNERS[…] ?? ''`, the
    /// empty-glob branch that renders the span with no SVG inside it — is
    /// deliberately *not* the one modelled, because the SVG is unanchored and
    /// that cell would compare identical to the resting one on every recorded
    /// field. A state flag whose cell cannot fail is the thing
    /// `Surface::unmodelled` exists to name.
    pub empty: bool,
}

impl FlickerSpinner {
    /// The captured cell: the agent chat pane's `reviving` spinner at a 1714px
    /// viewport — `24 × 24`, `bg #00000000`, `radius 0`, `border.w 0`,
    /// `visible: true`, and **no text group at all**.
    #[must_use]
    pub fn fixture() -> Self {
        Self {
            call_site: CallSite::ChatPane,
            empty: false,
        }
    }

    /// The box this cell paints, in logical px.
    #[must_use]
    pub fn size(self) -> Pixels {
        if self.empty {
            px(0.0)
        } else {
            self.call_site.extent()
        }
    }

    /// The span's box.
    ///
    /// `inline-flex` becomes `.flex()` for `badge`'s reason: gpui has no inline
    /// flow, and the live computed `display` is **`flex`** anyway — measured on
    /// the captured element, CSS having blockified a flex item's display.
    ///
    /// `items-center justify-center` centre the SVG the port does not draw, and
    /// `[&>svg]:size-full` sizes it; both are carried as the layout they are so
    /// that the box is the one the reference measures. No background, no radius,
    /// no border: `flicker-spinner.tsx` carries no `border` class, so
    /// preflight's `border: 0 solid` stands — `kbd`'s side of that trap.
    fn shell(self) -> Div {
        let extent = self.size();
        div()
            .flex()
            .items_center()
            .justify_center()
            .w(extent)
            .h(extent)
    }

    /// The element, with its one anchor.
    pub fn render(self, anchors: &dyn AnchorSink) -> AnyElement {
        anchors.boxed(AnchorId::from(ID_FLICKER_SPINNER), self.shell())
    }
}

#[cfg(test)]
mod tests {
    use super::{
        ALL_CALL_SITES, CallSite, FLICKER_PERIOD_RANGE, FlickerSpinner, ID_FLICKER_SPINNER,
        SIZE_3_5, SIZE_4, SIZE_6, SPINNER_VARIANTS,
    };
    use gpui::px;

    /// Neither declaration is made, and each for its own reason — see the
    /// constants.
    #[test]
    fn a_flicker_spinner_declares_neither_content_nor_line_sizing() {
        assert!(super::CONTENT_SIZED.is_empty());
        assert!(super::LINE_SIZED.is_empty());
    }

    /// Every live call site names a `size-*`, and one of them **restates the
    /// primitive's own** — which is a fact about the call site rather than a
    /// coincidence, so it is asserted rather than collapsed away.
    #[test]
    fn the_chat_glyph_restates_the_primitives_own_size() {
        assert_eq!(CallSite::ChatGlyph.extent(), CallSite::None.extent());
        assert_eq!(CallSite::None.extent(), SIZE_4);
        // And the other two are genuinely different numbers, so the assertion
        // above is not one that would pass whatever the sizes were.
        assert_eq!(CallSite::ChatPane.extent(), SIZE_6);
        assert_eq!(CallSite::WorkspaceIcon.extent(), SIZE_3_5);
        assert_ne!(SIZE_4, SIZE_6);
        assert_ne!(SIZE_4, SIZE_3_5);
    }

    /// §8.3's `empty`: a zero-area box, from every call site.
    #[test]
    fn empty_overrides_every_call_sites_box() {
        for call_site in ALL_CALL_SITES {
            let cell = FlickerSpinner {
                call_site,
                empty: true,
            };
            assert_eq!(cell.size(), px(0.0), "{}", call_site.name());
            // The control: the same call site has a box when it is not empty.
            assert!(
                FlickerSpinner {
                    call_site,
                    empty: false
                }
                .size()
                    > px(0.0),
                "{}",
                call_site.name(),
            );
        }
    }

    /// **No settling time exists for this component**, which is why the v1.9
    /// argument has to be about the animated *property*.
    ///
    /// The loop lengths straddle the rotating spinner's whole turn — 0.36s at
    /// the short end and 1.62s at the long — so there is no wait a capture could
    /// take that would put every variant at a known phase, and there are 31
    /// variants one of which is picked at random. The claim that matters is
    /// therefore the other one: nothing in flight is a recorded field.
    #[test]
    fn no_settling_wait_could_cover_every_variant() {
        let (shortest, longest) = FLICKER_PERIOD_RANGE;
        assert_eq!(shortest.as_millis(), 360);
        assert_eq!(longest.as_millis(), 1620);
        let turn = crate::primitives::spinner::PERIOD;
        assert!(
            shortest < turn && longest > turn,
            "the range straddles a turn"
        );
        assert_eq!(SPINNER_VARIANTS, 31);
    }

    /// The fixture is the captured cell, and the anchor is the one the reference
    /// names.
    #[test]
    fn the_fixture_is_the_captured_chat_pane_spinner() {
        let fixture = FlickerSpinner::fixture();
        assert_eq!(fixture.call_site, CallSite::ChatPane);
        assert!(!fixture.empty);
        assert_eq!(fixture.size(), px(24.0));
        assert_eq!(ID_FLICKER_SPINNER, "flicker-spinner");
    }

    /// The vocabulary is closed and its words are unique.
    #[test]
    fn the_call_site_vocabulary_is_closed() {
        let mut names: Vec<_> = ALL_CALL_SITES.iter().map(|c| c.name()).collect();
        names.sort_unstable();
        assert_eq!(names, ["chat-glyph", "chat-pane", "none", "workspace-icon"]);
    }
}
