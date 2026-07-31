//! `crowbar_mark` — the brand mark, and the first component in the port whose
//! **entire visible content is invisible to the contract**.
//!
//! The native half of `web/src/components/ui/crowbar-mark.tsx`: one `<svg>`
//! carrying a `146 × 145` viewBox, two `<path>`s and `fill="currentColor"`. It
//! has no class list at all, so there is nothing here to translate off one — the
//! numbers below are the *call site's*, measured live. See
//! `native/mapping/crowbar-mark.md`.
//!
//! # What the oracle can and cannot see, stated first because it is the point
//!
//! `native/oracle/ANCHORS.md` §3 records a **box**: `bounds`, `bg`, `visible`,
//! `radius`, `border` — plus a text group for an element with its own text
//! nodes. An `<svg>`'s paint is none of those. The mark's `fill` resolves
//! `currentColor` off the call site's `text-muted-foreground`, and the extractor
//! emits `fg` only from `oracleOwnText(el)`, which walks `el.childNodes` for
//! text nodes; `<path>` is an element, not a text node.
//!
//! So the anchor pins **bounds, `bg`, `visible`, `radius` and `border.w`** and
//! nothing else. Every pixel the mark actually paints — the ring, the crowbar
//! glyph, their colour — is outside the contract, exactly as `resizable`'s hit
//! strip and `button`'s `::before` overlay are. This module therefore draws an
//! **empty box** of the right extent, the same call every component since
//! `git_status_row` has made about icons: there is no native equivalent, and
//! drawing a substitute would put a shape on screen for the oracle to converge
//! on. That is a real visual gap and it is not one a passing gate would close.
//!
//! # The mark authors **no box**, so both axes are the call site's
//!
//! No `className`, no `width`/`height` attribute. There is exactly **one** live
//! call site, `features/tabs/components/tab-bar-item.tsx`, which passes
//! `size-[18px] shrink-0 text-muted-foreground` — an arbitrary-value class, so
//! the 18 is the *input* both engines resolve rather than either engine's
//! output. One call site is why there is no `CallSite` vocabulary here: a
//! one-word enum would be a knob with nothing to choose.
//!
//! **The bare primitive is deliberately not modelled.** An `<svg>` with a
//! viewBox and no size resolves through SVG's own `width: auto`, which `WebKit`
//! answers with the 300 × 150 default object size constrained by the viewBox
//! ratio — `151.03 × 150`. Nothing in the product renders that, and a port that
//! guessed zero there would compare a zero box against a 151px one. So the only
//! sizeless cell modelled is [`CrowbarMark::empty`], which pins **zero**
//! explicitly and is a rendering both engines agree on.
//!
//! # The viewBox is `146 × 145` — **not** square, and not a constant here
//!
//! The one call site's box *is* square, so the art is letterboxed inside it by
//! `preserveAspectRatio`'s default. Nothing in the contract can see that and the
//! port draws no art, so the two numbers live in this comment and in
//! `native/mapping/crowbar-mark.md` rather than as constants:
//! `sidebar_toggle_icon`'s rule, that a constant nothing draws from is a value
//! which can drift without anything noticing.
//!
//! # It is deliberately **larger than its slot**, and that is a layout to get right
//!
//! The slot is `grid size-3.5 place-content-center` — 14px — and the call site's
//! own comment says the overflow is intentional: "*Deliberately LARGER than the
//! 14px icon slot (it overflows the place-content-center box, which has no
//! clip)*". Measured live, the mark is `18 × 18` inside a `14 × 14` parent and
//! reports `visible: true`. [`CrowbarMark::in_slot`] renders that parent so the
//! overflow is exercised rather than asserted; the slot carries no anchor.

use gpui::{AnyElement, Div, IntoElement as _, ParentElement as _, Pixels, Styled as _, div, px};

use super::anchor::{AnchorId, AnchorSink};

/// The single anchor this surface carries.
pub const ID_CROWBAR_MARK: &str = "crowbar-mark";

/// **Nothing.** The mark paints no text and its box is a pinned extent the call
/// site names, not a content width — `ANCHORS.md` v1.5 does not apply.
pub const CONTENT_SIZED: [&str; 0] = [];

/// **Nothing.** No text, so no line box for a height to be derived from;
/// `ANCHORS.md` v1.6 makes the declaration valid only on an anchor carrying a
/// `font`.
pub const LINE_SIZED: [&str; 0] = [];

/// `size-[18px]` — the tab bar's extent, on both axes. Measured live at
/// `18 × 18`.
pub const TAB_BAR_EXTENT: Pixels = px(18.0);

/// `grid size-3.5 place-content-center` — the 14px slot the mark overflows.
///
/// `3.5 × --spacing` at the stock `0.25rem`, measured live as `14px`.
pub const TAB_BAR_SLOT: Pixels = px(14.0);

/// One `<CrowbarMark>`.
#[derive(Clone, Copy, Debug, PartialEq)]
pub struct CrowbarMark {
    /// Whether to draw the `grid size-3.5 place-content-center` slot around it,
    /// which is how the live tab bar renders it — and which the mark overflows.
    ///
    /// The slot carries **no anchor**: `data-oracle-id` lives on the primitive,
    /// and an id on a wrapper the primitive does not own would be an anchor the
    /// React side has no way to place.
    pub in_slot: bool,
    /// §8.3's `empty`: the mark with its extent merged away.
    ///
    /// Expressible from a call site — `cn` is tailwind-merge, so
    /// `className="size-0"` replaces `size-[18px]` — and taken by none. Both
    /// engines give a zero-extent box zero area and report `visible: false`, so
    /// the cell is comparable, which is why it is this and not "no className at
    /// all". See the module docs for what the bare primitive would actually do.
    pub empty: bool,
}

impl CrowbarMark {
    /// The captured cell: the tab bar's `newTab` icon at a 1714px viewport —
    /// `18 × 18`, `bg #00000000`, `radius 0`, `border.w 0`, `visible: true`.
    #[must_use]
    pub fn fixture() -> Self {
        Self {
            in_slot: false,
            empty: false,
        }
    }

    /// The extent this cell renders at: the call site's, or zero.
    #[must_use]
    pub fn extent(self) -> Pixels {
        if self.empty { px(0.0) } else { TAB_BAR_EXTENT }
    }

    /// The mark's own box: the call site's extent, and `shrink-0`.
    ///
    /// No background, no radius and no border. `crowbar-mark.tsx` names none,
    /// and Tailwind's preflight sets `border: 0 solid` on every element — the
    /// reference agrees, `border.w 0` against a `#ffffff0f` colour the cascade
    /// resolved and nothing paints (`ANCHORS.md` v1.3 ruling 2 compares that
    /// field only above zero width).
    fn shell(self) -> Div {
        let extent = self.extent();
        div().flex_shrink_0().w(extent).h(extent)
    }

    /// The slot the tab bar draws around the mark — 14px, centring its content
    /// and clipping nothing.
    fn slot() -> Div {
        div()
            .flex()
            .items_center()
            .justify_center()
            .flex_shrink_0()
            .w(TAB_BAR_SLOT)
            .h(TAB_BAR_SLOT)
    }

    /// The element, with its one anchor.
    pub fn render(self, anchors: &dyn AnchorSink) -> AnyElement {
        let mark = anchors.boxed(AnchorId::from(ID_CROWBAR_MARK), self.shell());
        if self.in_slot {
            Self::slot().child(mark).into_any_element()
        } else {
            mark
        }
    }
}

#[cfg(test)]
mod tests {
    use super::{CrowbarMark, TAB_BAR_EXTENT, TAB_BAR_SLOT};
    use gpui::px;

    /// Neither declaration is made, and each for its own reason — see the
    /// constants. Asserted rather than left implicit: a list that grew by
    /// accident is the silent divergence `ANCHORS.md` v1.5 and v1.6 exist to
    /// prevent.
    #[test]
    fn a_mark_declares_neither_content_nor_line_sizing() {
        assert!(super::CONTENT_SIZED.is_empty());
        assert!(super::LINE_SIZED.is_empty());
    }

    /// The fixture is the captured cell, and its extent is the call site's 18 —
    /// the mark authors none of its own.
    #[test]
    fn the_fixture_is_the_captured_tab_bar_icon() {
        let fixture = CrowbarMark::fixture();
        assert!(!fixture.in_slot);
        assert!(!fixture.empty);
        assert_eq!(fixture.extent(), TAB_BAR_EXTENT);
        assert_eq!(TAB_BAR_EXTENT, px(18.0));
    }

    /// §8.3's `empty` pins **zero**, which both engines agree has no area —
    /// unlike the bare primitive, whose SVG default this port deliberately does
    /// not model. The slot does not rescue it: an empty mark inside the 14px
    /// slot is still a zero box.
    #[test]
    fn the_empty_cell_pins_a_zero_extent() {
        for in_slot in [false, true] {
            let empty = CrowbarMark {
                in_slot,
                empty: true,
            };
            assert_eq!(empty.extent(), px(0.0), "in_slot {in_slot}");
        }
    }

    /// The extent **overflows the slot**, which is the call site's stated
    /// intent and the one layout fact on this surface a port could get wrong by
    /// "normalising" it. A control as much as an assertion: were the two equal,
    /// nothing here would be testing the overflow.
    #[test]
    fn the_mark_is_larger_than_the_slot_it_sits_in() {
        assert_eq!(TAB_BAR_SLOT, px(14.0));
        assert!(
            TAB_BAR_EXTENT > TAB_BAR_SLOT,
            "the tab bar's mark overflows its 14px slot on purpose: {TAB_BAR_EXTENT:?} vs {TAB_BAR_SLOT:?}",
        );
    }
}
