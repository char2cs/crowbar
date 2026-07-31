//! `crowbar_wordmark` — the brand lockup, and the port's first **intrinsic
//! aspect ratio**.
//!
//! The native half of `web/src/components/ui/crowbar-wordmark.tsx`: one `<svg>`
//! carrying a `453 × 115` viewBox, thirty-one `<path>`s and
//! `fill="currentColor"`. Like [`crowbar_mark`](super::crowbar_mark) it has no
//! class list of its own, so every number below is a call site's, measured live.
//! See `native/mapping/crowbar-wordmark.md`.
//!
//! # What the oracle can and cannot see
//!
//! The same answer as the mark's, and worth repeating rather than
//! cross-referencing because the lettering makes it *look* like a text run:
//! **it is not one.** "Crowbar" is `<path>` fill, so the extractor's
//! `oracleOwnText` finds no text node, and the anchor carries no `text`,
//! `text_width`, `clipped`, `font` or `fg` — the whole group is absent, not
//! empty. What the contract compares here is `bounds`, `bg`, `visible`,
//! `radius` and `border.w`, and that is the complete list.
//!
//! This module therefore draws an **empty box** of the right extent. The
//! lettering is a real visual gap the oracle cannot close either way; it is
//! `git_status_row`'s call about icons, applied to the largest piece of art in
//! the product.
//!
//! # The height is the **viewBox's ratio**, and it is the one derived number here
//!
//! Every live call site pins a width and leaves `h-auto`, so the height comes
//! from the replaced element's intrinsic ratio: `width × 115 / 453`. That is
//! not a content width and not a line box, so neither `ANCHORS.md` v1.5 nor v1.6
//! applies — it is a third way for a box to get a size, and the first in this
//! port.
//!
//! ## A systematic difference, measured rather than inferred
//!
//! Both engines quantise the ratio, and they quantise it differently:
//!
//! | | captured cell |
//! |---|---|
//! | exact `148 × 115 / 453` | `37.5717` |
//! | **`WebKit`** — floors into `1/64`px `LayoutUnit`s | **`37.5625`** (`2404/64`) |
//! | **gpui** — snaps to the device pixel grid, `0.5`px at DPR 2 | **`37.5`** |
//!
//! A **`0.0625`** delta against `ANCHORS.md` §5's ±0.5. Measured in the harness,
//! not derived: the port hands taffy the exact `f32` and reads `37.5` back.
//!
//! **The bound is DPR-dependent, and at DPR 1 it grazes the tolerance.**
//! `|snap(L) − L| ≤ 1/(2·dpr)` and `|L − floor₆₄(L)| ≤ 1/64`, so the worst case
//! is `1/(2·dpr) + 1/64`: `0.266` at DPR 2 and `0.516` at DPR 1 — just over ±0.5,
//! for a width whose exact height lands the two quantisations at opposite ends.
//! Nothing today gets near it (the live clamp only ever produces 96 … 148, and
//! the archived runs are DPR 2), and this is a property of the two engines
//! rather than a defect in the port. It is written down here and in
//! `native/mapping/crowbar-wordmark.md` rather than papered over by pinning the
//! reference's number in the component, which `ANCHORS.md` rejects by name.
//!
//! There is no declaration for it, and none is proposed. v1.5 and v1.6 exist
//! because one engine transforms a quantity the other keeps; here **both**
//! transform, and v1.6 is explicit that inventing a rule for that case would
//! make the differ compare something it recomputed rather than what the
//! extractors measured.
//!
//! # The widths are `clamp()` and `min()` over a length the *container* supplies
//!
//! `new-tab-view.tsx` asks for `w-[clamp(96px,14cqmin,148px)]` inside a
//! `container-type: size` pane, and the two OOBE lockups ask for
//! `w-[min(360px,44vw)]` and `w-[min(180px,28vw)]`. [`CallSite::width`] takes
//! the length each resolves against — the pane's shorter side, or the viewport
//! width — and resolves the arithmetic. That is P3.1's line held: the knob
//! supplies the same *input* both engines resolve, never the reference's
//! output. Measured on the captured cell: the pane is `1417 × 1073`, so
//! `14cqmin` is `150.22` and the clamp's **upper bound binds** at 148.

use gpui::{AnyElement, Div, Pixels, Styled as _, div, px};

use super::anchor::{AnchorId, AnchorSink};

/// The single anchor this surface carries.
pub const ID_CROWBAR_WORDMARK: &str = "crowbar-wordmark";

/// **Nothing.** The wordmark paints no text — the lettering is path fill — and
/// its width is a call site's `clamp()`, not a content width.
pub const CONTENT_SIZED: [&str; 0] = [];

/// **Nothing.** No text, so no line box. The height is the viewBox's ratio,
/// which `ANCHORS.md` has no declaration for and needs none: it is compared as
/// an ordinary `bounds.h`.
pub const LINE_SIZED: [&str; 0] = [];

/// The viewBox's width, `453`.
pub const VIEW_BOX_WIDTH: f32 = 453.0;

/// The viewBox's height, `115`.
pub const VIEW_BOX_HEIGHT: f32 = 115.0;

/// `clamp(96px, …, 148px)`'s floor, from `new-tab-view.tsx`.
pub const NEW_TAB_MIN_WIDTH: Pixels = px(96.0);

/// `clamp(…, …, 148px)`'s ceiling. **This is the bound that binds on the
/// captured cell** — the pane's shorter side is 1073, so `14cqmin` overshoots.
pub const NEW_TAB_MAX_WIDTH: Pixels = px(148.0);

/// `14cqmin` as a fraction of the container's shorter side.
pub const NEW_TAB_CONTAINER_FRACTION: f32 = 0.14;

/// `min(360px, …)` — the OOBE presentation lockup's ceiling.
pub const OOBE_PRESENTATION_MAX_WIDTH: Pixels = px(360.0);

/// `44vw` as a fraction of the viewport width.
pub const OOBE_PRESENTATION_VIEWPORT_FRACTION: f32 = 0.44;

/// `min(180px, …)` — the OOBE card lockup's ceiling.
pub const OOBE_CARD_MAX_WIDTH: Pixels = px(180.0);

/// `28vw` as a fraction of the viewport width.
pub const OOBE_CARD_VIEWPORT_FRACTION: f32 = 0.28;

/// The `className` bundle a call site merges. The wordmark has none of its own,
/// so this is the whole of its box.
///
/// **There is no `None` variant**, and the omission is deliberate: an `<svg>`
/// with a viewBox and no size resolves through SVG's own `width: auto`, which
/// `WebKit` answers with the 300 × 150 default object size constrained by the
/// viewBox ratio — `453 × 115` giving `453 × 115.01` at 300 wide, clamped to
/// 150 tall. Nothing in the product renders that, and modelling it as a zero
/// box would compare zero against a three-figure number. The sizeless cell that
/// *is* modelled is [`CrowbarWordmark::empty`], which pins zero explicitly.
#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub enum CallSite {
    /// `new-tab-view.tsx`'s isologo:
    /// `pointer-events-none h-auto w-[clamp(96px,14cqmin,148px)] text-muted-foreground`.
    /// **The captured cell.**
    NewTabView,
    /// `oobe-screen.tsx`'s full-screen lockup, `w-[min(360px,44vw)] text-white`.
    ///
    /// **Not reachable, and not for want of trying** — see the surface docs.
    OobePresentation,
    /// `oobe-screen.tsx`'s card lockup, `w-[min(180px,28vw)] text-white`. Also
    /// unreachable.
    OobeCard,
}

/// Every modelled call site, for `--help` and the closed-vocabulary test.
pub const ALL_CALL_SITES: [CallSite; 3] = [
    CallSite::NewTabView,
    CallSite::OobePresentation,
    CallSite::OobeCard,
];

impl CallSite {
    /// The word `--call-site` takes.
    #[must_use]
    pub fn name(self) -> &'static str {
        match self {
            Self::NewTabView => "new-tab-view",
            Self::OobePresentation => "oobe-presentation",
            Self::OobeCard => "oobe-card",
        }
    }

    /// The width this call site resolves to, given the length its
    /// container-relative term measures against.
    ///
    /// `basis` is the **pane's shorter side** for [`Self::NewTabView`] (`cqmin`
    /// in a `container-type: size` ancestor) and the **viewport width** for the
    /// two OOBE lockups (`vw`). One parameter rather than two because each call
    /// site has exactly one such term, and a second would be a length no class
    /// here reads.
    #[must_use]
    pub fn width(self, basis: Pixels) -> Pixels {
        let basis = f32::from(basis);
        match self {
            Self::NewTabView => px((basis * NEW_TAB_CONTAINER_FRACTION)
                .clamp(f32::from(NEW_TAB_MIN_WIDTH), f32::from(NEW_TAB_MAX_WIDTH))),
            Self::OobePresentation => px((basis * OOBE_PRESENTATION_VIEWPORT_FRACTION)
                .min(f32::from(OOBE_PRESENTATION_MAX_WIDTH))),
            Self::OobeCard => {
                px((basis * OOBE_CARD_VIEWPORT_FRACTION).min(f32::from(OOBE_CARD_MAX_WIDTH)))
            }
        }
    }
}

/// The height a wordmark of this width takes, from the viewBox's ratio.
///
/// `h-auto` on a replaced element with an intrinsic ratio and a definite width.
/// The **exact** ratio: taffy rounds it to a whole pixel at layout time and
/// `WebKit` floors it into `1/64`ths, and neither quantisation belongs in the
/// authored value. See the module docs for what the two cost on the captured
/// cell.
#[must_use]
pub fn height_for(width: Pixels) -> Pixels {
    px(f32::from(width) * VIEW_BOX_HEIGHT / VIEW_BOX_WIDTH)
}

/// One `<CrowbarWordmark>`.
#[derive(Clone, Copy, Debug, PartialEq)]
pub struct CrowbarWordmark {
    /// The call site whose className supplies the width.
    pub call_site: CallSite,
    /// The length that call site's container-relative term resolves against —
    /// the pane's shorter side, or the viewport width. See [`CallSite::width`].
    pub basis: Pixels,
    /// §8.3's `empty`: the lockup with its width merged away.
    ///
    /// Expressible from a call site — `cn` is tailwind-merge, so a
    /// `className="size-0"` replaces the arbitrary width — and taken by none.
    /// Both engines give a zero box zero area and report `visible: false`, so
    /// the cell is comparable, unlike the bare primitive's SVG default. See
    /// [`CallSite`] for why that is not a variant.
    pub empty: bool,
}

/// The captured cell's pane: `1417 × 1073`, so `cqmin` is **1073**.
///
/// A `u16` rather than `Pixels` because it reaches the surface's `Params`, which
/// reaches `Cell`, which has to stay `Eq` — `f32` is not, and a container side
/// with a fractional part is not a thing anyone measures.
pub const CAPTURED_PANE_MIN_SIDE: u16 = 1073;

impl CrowbarWordmark {
    /// The captured cell: the new-tab pane's isologo at a 1714px viewport in a
    /// `1417 × 1073` pane — `148 × 37.56`, `bg #00000000`, `radius 0`,
    /// `border.w 0`, `visible: true`.
    #[must_use]
    pub fn fixture() -> Self {
        Self {
            call_site: CallSite::NewTabView,
            basis: px(f32::from(CAPTURED_PANE_MIN_SIDE)),
            empty: false,
        }
    }

    /// This cell's resolved width — the call site's clamp, or zero.
    #[must_use]
    pub fn width(self) -> Pixels {
        if self.empty {
            px(0.0)
        } else {
            self.call_site.width(self.basis)
        }
    }

    /// The wordmark's box.
    ///
    /// No background, no radius, no border: the file names none and preflight
    /// zeroes `border` for every element. The reference agrees —
    /// `border.w 0`, and `ANCHORS.md` v1.3 ruling 2 ignores the colour there.
    fn shell(self) -> Div {
        let width = self.width();
        div().w(width).h(height_for(width))
    }

    /// The element, with its one anchor.
    pub fn render(self, anchors: &dyn AnchorSink) -> AnyElement {
        anchors.boxed(AnchorId::from(ID_CROWBAR_WORDMARK), self.shell())
    }
}

#[cfg(test)]
mod tests {
    use super::{
        ALL_CALL_SITES, CAPTURED_PANE_MIN_SIDE, CallSite, CrowbarWordmark, NEW_TAB_MAX_WIDTH,
        NEW_TAB_MIN_WIDTH, OOBE_CARD_MAX_WIDTH, OOBE_PRESENTATION_MAX_WIDTH, height_for,
    };
    use gpui::px;

    /// Neither declaration is made — see the constants.
    #[test]
    fn a_wordmark_declares_neither_content_nor_line_sizing() {
        assert!(super::CONTENT_SIZED.is_empty());
        assert!(super::LINE_SIZED.is_empty());
    }

    /// **The captured cell reproduces from its inputs**: a `1073`px shorter
    /// side gives `14cqmin = 150.22`, the clamp's ceiling binds at 148, and the
    /// ratio gives a height within `ANCHORS.md` §5's ±0.5 of the reference's
    /// `37.56`.
    ///
    /// The width is asserted exactly and the height within tolerance, which is
    /// the honest split: the clamp is integer arithmetic both engines do the
    /// same way, and the height is where the two quantisations part. This
    /// asserts the **authored** value; what taffy then rounds it to is measured
    /// in `crates/crowbar-app/src/row_layout/crowbar_wordmark.rs`, where there
    /// is a window.
    #[test]
    fn the_captured_cell_falls_out_of_the_clamp_and_the_ratio() {
        let fixture = CrowbarWordmark::fixture();
        assert_eq!(fixture.basis, px(f32::from(CAPTURED_PANE_MIN_SIDE)));
        assert!(!fixture.empty);

        let width = fixture.width();
        assert_eq!(width, NEW_TAB_MAX_WIDTH);

        // The middle term really did overshoot, so the ceiling is doing work
        // rather than coinciding with it.
        let unclamped = f32::from(CAPTURED_PANE_MIN_SIDE) * super::NEW_TAB_CONTAINER_FRACTION;
        assert!(
            unclamped > f32::from(NEW_TAB_MAX_WIDTH),
            "14cqmin should overshoot the 148px ceiling, got {unclamped}",
        );

        let height = f32::from(height_for(width));
        assert!(
            (height - 37.56).abs() <= 0.5,
            "the reference reports 37.56, the ratio gives {height}",
        );
    }

    /// The clamp binds at **both** ends, which is what makes it a clamp rather
    /// than a maximum. A narrow pane floors the lockup at 96 and a wide one
    /// caps it at 148.
    #[test]
    fn the_new_tab_clamp_binds_at_both_ends() {
        assert_eq!(CallSite::NewTabView.width(px(100.0)), NEW_TAB_MIN_WIDTH);

        // A pane whose shorter side lands the middle term strictly between the
        // two bounds: 800 × 0.14 = 112.
        let middle = CallSite::NewTabView.width(px(800.0));
        assert!(
            middle > NEW_TAB_MIN_WIDTH && middle < NEW_TAB_MAX_WIDTH,
            "{middle:?}"
        );
        assert!((f32::from(middle) - 112.0).abs() < 1e-3, "{middle:?}");

        assert_eq!(CallSite::NewTabView.width(px(5000.0)), NEW_TAB_MAX_WIDTH);
    }

    /// The two OOBE lockups are `min()`, not `clamp()`: they have a ceiling and
    /// **no floor**, so a narrow window shrinks them without limit.
    #[test]
    fn the_oobe_lockups_have_a_ceiling_and_no_floor() {
        for (call_site, ceiling, fraction) in [
            (
                CallSite::OobePresentation,
                OOBE_PRESENTATION_MAX_WIDTH,
                super::OOBE_PRESENTATION_VIEWPORT_FRACTION,
            ),
            (
                CallSite::OobeCard,
                OOBE_CARD_MAX_WIDTH,
                super::OOBE_CARD_VIEWPORT_FRACTION,
            ),
        ] {
            // A real Crowbar window is far past the ceiling.
            assert_eq!(call_site.width(px(1714.0)), ceiling, "{}", call_site.name());
            // And a narrow one is the fraction, with nothing catching it.
            let narrow = call_site.width(px(200.0));
            assert!(
                (f32::from(narrow) - 200.0 * fraction).abs() < 1e-3,
                "{}: {narrow:?}",
                call_site.name(),
            );
        }
    }

    /// §8.3's `empty` pins **zero** on both axes, whatever the call site and
    /// whatever the basis — the cell both engines agree has no area.
    #[test]
    fn the_empty_cell_pins_a_zero_box() {
        for call_site in ALL_CALL_SITES {
            let empty = CrowbarWordmark {
                call_site,
                basis: px(1073.0),
                empty: true,
            };
            assert_eq!(empty.width(), px(0.0), "{}", call_site.name());
            assert_eq!(height_for(empty.width()), px(0.0), "{}", call_site.name());
        }
    }

    /// The ratio is the viewBox's and nothing else — a wordmark twice as wide
    /// is twice as tall.
    #[test]
    fn the_height_is_the_view_boxs_ratio() {
        let one = f32::from(height_for(px(453.0)));
        assert!((one - 115.0).abs() < 1e-3, "{one}");
        let two = f32::from(height_for(px(906.0)));
        assert!((two - 230.0).abs() < 1e-3, "{two}");
    }

    /// The vocabulary is closed and its words are unique.
    #[test]
    fn the_call_site_vocabulary_is_closed() {
        let mut names: Vec<_> = ALL_CALL_SITES.iter().map(|c| c.name()).collect();
        names.sort_unstable();
        assert_eq!(names, ["new-tab-view", "oobe-card", "oobe-presentation"]);
        assert_eq!(CrowbarWordmark::fixture().call_site, CallSite::NewTabView);
    }
}
