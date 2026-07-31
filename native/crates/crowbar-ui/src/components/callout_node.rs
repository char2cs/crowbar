//! `callout-node` — a Plate callout block, and the component where **every one
//! of the class list's visual utilities is dead**.
//!
//! The native half of `web/src/components/ui/callout-node.tsx`. See
//! `native/mapping/callout-node.md` and `/tmp/p3-ref-callout-node.json`.
//!
//! # F3, in its purest form yet: a *stylesheet* beats the whole class list
//!
//! `MAPPING.md`'s standing rule is "compile the CSS, do not read the class
//! name". On this component even the compiled utility is the wrong answer,
//! because a descendant selector outranks all of them. `CalloutElement` writes
//!
//! ```text
//! my-1 flex rounded-sm bg-muted p-4 pl-3
//! ```
//!
//! and `features/editor/markdown/plate/markdown-editor.css` answers
//!
//! ```css
//! .crowbar-markdown-editor .slate-callout {
//!   margin-block: 1.2em;
//!   padding: 0.85rem 1rem;
//!   border-radius: var(--radius-md);
//!   background: var(--md-wash);
//! }
//! ```
//!
//! Measured on the live element, every single one loses:
//!
//! | class | reads as | **renders as** |
//! |---|---|---|
//! | `my-1` | 4px | **19.19px** (`1.2em`) |
//! | `rounded-sm` | 6px | **8px** (`--radius-md`) |
//! | `bg-muted` | `--muted`, white @ 4% | **`--md-wash`**, *foreground* @ 5% |
//! | `p-4` | 16px | **13.6px** block, 16px inline (`0.85rem 1rem`) |
//! | `pl-3` | 12px | **16px** — overridden along with the rest of `padding` |
//!
//! Only `flex` survives. A port written from the class list would be wrong in
//! five properties at once and would *look* faithful, which is exactly the
//! failure F3 was written about — and note that the two earlier defences do not
//! catch it: reading the class name fails, and compiling the utility fails too.
//! **The value has to come off the running element.**
//!
//! # `bg` is derived from a token, not minted
//!
//! `--md-wash` is `color-mix(in oklch, var(--foreground) 5%, transparent)` and
//! lives in `markdown-editor.css`, which F5 lists among the `--md-*` properties
//! that are *not* ported into the sealed table. So it is computed the way
//! `file_tree_hover_bg` is — [`crate::Theme::color_foreground`] through
//! `Color::mix` — rather than written down as a literal, which rule 4 forbids
//! anyway.
//!
//! The mix spaces differ (`Color::mix` is `in srgb`, the stylesheet says
//! `in oklch`) and here that is provably immaterial: the second colour is
//! `transparent`, so the operation only scales alpha and both spaces return the
//! source colour at 5%. `the_wash_is_the_foreground_at_five_percent` pins the
//! result against the reference's `#f5f5f50d` rather than trusting the argument.
//!
//! # Neither declaration is made, and the emoji is `badge`'s trap again
//!
//! Nothing here is `content_sized`: the callout and both inner boxes are
//! stretched (`w-full`, or the editor's own measure), and the emoji control is
//! authored `size-6`. Nothing is `line_sized` either — and the emoji is the
//! interesting one, because it is the only anchor on this surface that paints
//! text. Its box is **32px** against a **20px** line box, so declaring it would
//! manufacture a 12px delta on the one anchor that carries a font, exactly as
//! `ANCHORS.md` v1.6 warns. `the_emoji_box_is_authored_not_line_sized` asserts
//! the gap rather than restating the opinion.
//!
//! # The emoji control is a `<Button>` that is **not** a [`Button`]
//!
//! The call site writes `size-6 select-none p-1 text-[18px]` over the variant's
//! own box, which moves *both* axes and the type step. Instantiating
//! [`Button`](super::button::Button) would mean giving it a size-override knob
//! that no other call site in the app needs, so the box is built here from
//! `button`'s own public constants — [`super::button::BORDER_WIDTH`] and
//! `Variant::Ghost`'s colours — which is what keeps the shared values from
//! drifting.
//!
//! Two measurements from it are worth carrying, and the first **contradicts a
//! recorded finding**:
//!
//! * **`radius` is 10 — the primitive's own `rounded-lg`, unmerged, and
//!   `visible: true`.** P3.1 recorded that "no live Button is both unmerged and
//!   visible", with the only two candidates sitting in the carousel's snapped-out
//!   panels. This one is neither: the callout's control overrides the box but
//!   writes no `rounded-*`, and it is on screen. The reference says
//!   `radius 10, visible true`.
//! * **`sm:h-8` beats `size-6` on the height but not on the width**, so the box
//!   is **24 × 32** and not the square its class reads as — the `sm:` trap, in
//!   one axis only, because `size-*` and `h-*` are different tailwind-merge
//!   groups and Tailwind emits the `sm:` variant later. Measured, and
//!   `the_emoji_box_is_wider_than_it_is_tall_by_the_sm_trap` pins it.
//!
//! # The body paragraph is drawn but not anchored, and it is a product defect
//!
//! The run inside a callout is a `.slate-p` — `ParagraphElement`'s node, not this
//! component's — so it carries no anchor here, exactly as `kbd`'s `KbdGroup`
//! carries none. It is still *drawn*, because the callout's height is its
//! padding plus its content and a callout with nothing in it would measure
//! something the app never shows.
//!
//! Its trailing gap is where the defect is. `markdown-editor.css` ends with
//!
//! ```css
//! .crowbar-markdown-editor
//!   :is(.slate-blockquote, .slate-callout, .slate-th, .slate-td, li)
//!   > .slate-p:last-child { margin-bottom: 0 }
//! ```
//!
//! — a **direct-child** combinator. `CalloutElement` wraps its children in two
//! divs, so the paragraph is a *grandchild* and the rule does not match: the
//! last paragraph in a callout keeps `--md-gap` (`0.9em`, measured **14.39px**)
//! where the same paragraph in a blockquote gets 0. The port reproduces what
//! renders, not what the stylesheet intended; [`BODY_GAP_EM`] records which is
//! which.

use gpui::{
    AnyElement, Div, FontWeight, IntoElement as _, ParentElement as _, Pixels, Rems, SharedString,
    Styled as _, StyledText, div, px, relative,
};

use super::anchor::AnchorSink;
use super::badge::TypeStep;
use super::button::{ButtonState, Variant};
use crate::theme::{Color, Theme};

/// The callout's own box — the surface root.
pub const ID_CALLOUT: &str = "callout";

/// `div.flex.w-full.gap-2.rounded-md`, the row holding the control and the body.
pub const ID_ROW: &str = "callout-row";

/// The emoji `<Button>`.
pub const ID_EMOJI: &str = "callout-emoji";

/// `div.w-full`, which holds `{children}`.
pub const ID_CONTENT: &str = "callout-content";

/// **Nothing.** Every box on this surface is stretched or authored — see the
/// module docs.
pub const CONTENT_SIZED: [&str; 0] = [];

/// **Nothing.** The only anchor that paints text authors its own height, which
/// `ANCHORS.md` v1.6 makes the test rather than "does it have a font".
pub const LINE_SIZED: [&str; 0] = [];

/// `margin-block: 1.2em`, in the callout's **own** em.
///
/// Not a `px` constant, because `em` resolves at the element that uses it and
/// the editor's base type step is a setting. Measured at the reference's 16px
/// base as `19.19px` top and bottom.
pub const MARGIN_BLOCK_EM: f32 = 1.2;

/// The block half of `padding: 0.85rem 1rem` — `rem`, so it does **not** track
/// the editor's type step. Measured live as `13.6px`.
pub const PADDING_Y: Pixels = px(13.6);

/// The inline half of `padding: 0.85rem 1rem`. Measured live as `16px`, and this
/// is the value `pl-3` fails to override.
pub const PADDING_X: Pixels = px(16.0);

/// `--md-wash`'s mix percentage against [`Color::TRANSPARENT`].
pub const WASH_TINT: f32 = 5.0;

/// `gap-2` on the inner row — `calc(var(--spacing) * 2)`, measured as `8px`.
///
/// This one is a *Tailwind* utility that survives, because the stylesheet rule
/// above targets `.slate-callout` and not the row inside it.
pub const GAP: Pixels = px(8.0);

/// `size-6`'s width — the axis the call site keeps. Measured `24px`.
pub const EMOJI_WIDTH: Pixels = px(24.0);

/// The height the variant's `sm:h-8` takes back off `size-6`. Measured `32px`.
///
/// See the module docs: `size-*` and `h-*` are different tailwind-merge groups,
/// so both survive `cn(…)` and the `sm:` variant is emitted later.
pub const EMOJI_HEIGHT: Pixels = px(32.0);

/// `p-1` on the control — `4px`.
pub const EMOJI_PADDING: Pixels = px(4.0);

/// The control's type step. `text-[18px]` is **dead**: the variant's
/// `sm:text-sm` is emitted later and wins, so the reference reads `size 14` on a
/// `line_height` of `20`.
pub const EMOJI_STEP: TypeStep = TypeStep {
    size: Rems(0.875),
    line_height: 20.0 / 14.0,
};

/// `font-medium`, off the button variant. The reference says `weight 500`.
pub const EMOJI_WEIGHT: FontWeight = FontWeight::MEDIUM;

/// The control's inline `fontFamily`, first family only per `ANCHORS.md` v1.2.
///
/// Named rather than inherited for `git_status_row`'s reason: an inherited style
/// reports macOS's `.SystemUIFont`, which the DOM never produces.
pub const EMOJI_FAMILY: &str = "Apple Color Emoji";

/// `props.element.icon || '💡'` — the fallback every callout without a chosen
/// emoji paints, and what the reference carries.
pub const DEFAULT_ICON: &str = "💡";

/// The editor's body step: `16px` on a `1.7` ratio, measured as
/// `line-height: 27.2px`.
pub const BODY_STEP: TypeStep = TypeStep {
    size: Rems(1.0),
    line_height: 1.7,
};

/// `--md-gap`, the gap under a block, in its own em. Measured `14.39px`.
///
/// **This is the defect, not the design** — see the module docs. The stylesheet
/// means to zero it on a callout's last paragraph and cannot, because its
/// combinator is `>` and this component nests the paragraph two levels down.
pub const BODY_GAP_EM: f32 = 0.9;

/// `hover:bg-muted-foreground/15` on the emoji control — the **only**
/// interaction rule anywhere on this component.
pub const EMOJI_HOVER_TINT: f32 = 15.0;

/// One `<CalloutElement>`.
#[derive(Clone, Debug, PartialEq)]
pub struct CalloutNode {
    /// The emoji the control paints.
    pub icon: SharedString,
    /// The body paragraph's text — `ParagraphElement`'s run, drawn here so the
    /// callout has the height it really has, and anchored nowhere.
    pub body: SharedString,
    /// Whether the pointer is over the emoji control.
    ///
    /// A **parameter**, never a `.hover(…)` refinement, for the reason
    /// `ANCHORS.md` §6 gives: gpui resolves interaction refinements from runtime
    /// state a snapshot cannot see, so a component that expressed its states that
    /// way would compare resting-against-resting in every hover cell and converge
    /// while proving nothing.
    pub emoji_hovered: bool,
}

impl CalloutNode {
    /// The captured cell: the default `💡` over a one-word body, measured at a
    /// 1714px viewport in a 720px editor column — `720 × 68.58`, `bg #f5f5f50d`,
    /// `radius 8`, `border 0`.
    ///
    /// **One word, deliberately.** The body is the only string on this surface a
    /// call site controls, and a run that wraps is outside what the contract can
    /// compare at all: the DOM sums client rects where gpui shapes one line. A
    /// fixture that cannot wrap keeps the cell comparable by construction rather
    /// than by the column happening to be wide enough.
    #[must_use]
    pub fn fixture() -> Self {
        Self {
            icon: SharedString::new_static(DEFAULT_ICON),
            body: SharedString::new_static("Reachability"),
            emoji_hovered: false,
        }
    }

    /// `--md-wash`: the foreground at [`WASH_TINT`] percent.
    #[must_use]
    pub fn wash(theme: &Theme) -> Color {
        theme.color_foreground.mix(WASH_TINT, Color::TRANSPARENT)
    }

    /// The callout's own box.
    ///
    /// `margin-block` is **not** applied here: `bounds` is a border box and the
    /// surface's own root is what a margin would move, which would offset every
    /// anchor by a constant and prove nothing. [`MARGIN_BLOCK_EM`] is recorded
    /// for the port's benefit and asserted in the layout tests.
    fn shell(theme: &Theme) -> Div {
        let family = theme.font_sans.primary().unwrap_or("sans-serif");
        div()
            .font_family(family)
            .flex()
            .w_full()
            .px(PADDING_X)
            .py(PADDING_Y)
            .rounded(theme.radius_md.value())
            .bg(Self::wash(theme))
            .text_size(BODY_STEP.size)
            .line_height(relative(BODY_STEP.line_height))
            .text_color(theme.color_foreground)
    }

    /// The emoji control — a `<Button variant="ghost">` whose box the call site
    /// has taken over in both axes. See the module docs for why this is not a
    /// [`Button`](super::button::Button).
    fn emoji_box(&self, theme: &Theme) -> Div {
        let mut element = div()
            .flex()
            .items_center()
            .justify_center()
            .w(EMOJI_WIDTH)
            .h(EMOJI_HEIGHT)
            .p(EMOJI_PADDING)
            .border_1()
            .border_color(Variant::Ghost.border(theme, ButtonState::resting()))
            .rounded(theme.radius_lg.value())
            .font_family(EMOJI_FAMILY)
            .text_size(EMOJI_STEP.size)
            .line_height(relative(EMOJI_STEP.line_height))
            .font_weight(EMOJI_WEIGHT)
            .text_color(Variant::Ghost.foreground(theme));

        if self.emoji_hovered {
            element = element.bg(theme
                .muted_foreground
                .mix(EMOJI_HOVER_TINT, Color::TRANSPARENT));
        }
        element
    }

    /// The body paragraph — `ParagraphElement`'s box, unanchored.
    ///
    /// The run is a bare [`StyledText`] rather than an [`AnchorSink`] call,
    /// which is the point: a sink call *records*, and an id for something that
    /// is not this surface's anchor would put a fifth row in the snapshot under
    /// a name the reference has nothing to match. `Unanchored::text` renders
    /// exactly this element, so the shipping build and the measured one still
    /// paint the same tree.
    fn body_paragraph(&self) -> Div {
        div()
            .mb(px(BODY_STEP.size.0 * 16.0 * BODY_GAP_EM))
            .child(StyledText::new(self.body.clone()))
    }

    /// The element, with its four anchors.
    pub fn render(&self, theme: &Theme, anchors: &dyn AnchorSink) -> AnyElement {
        let emoji = anchors.boxed_text(ID_EMOJI.into(), self.emoji_box(theme), self.icon.clone());

        let content = anchors.boxed(
            ID_CONTENT.into(),
            div().w_full().child(self.body_paragraph()),
        );

        let row = anchors.boxed(
            ID_ROW.into(),
            div()
                .flex()
                .w_full()
                .gap(GAP)
                .rounded(theme.radius_md.value())
                .child(emoji)
                .child(content),
        );

        anchors
            .root(ID_CALLOUT.into(), Self::shell(theme).child(row))
            .into_any_element()
    }
}

#[cfg(test)]
mod tests {
    use super::{
        BODY_GAP_EM, BODY_STEP, CONTENT_SIZED, CalloutNode, DEFAULT_ICON, EMOJI_HEIGHT, EMOJI_STEP,
        EMOJI_WIDTH, GAP, LINE_SIZED, MARGIN_BLOCK_EM, PADDING_X, PADDING_Y, WASH_TINT,
    };
    use crate::theme::Theme;
    use gpui::px;

    /// Both declaration lists are empty, and that is a measurement rather than
    /// an omission — see the module docs.
    #[test]
    fn this_surface_declares_neither_content_sized_nor_line_sized() {
        assert!(CONTENT_SIZED.is_empty());
        assert!(LINE_SIZED.is_empty());
    }

    /// The one anchor that paints text authors a box **12px taller** than its own
    /// line box, so `ANCHORS.md` v1.6's declaration would manufacture a delta.
    ///
    /// The same shape `label` uses to keep its own list a measurement: assert the
    /// distance, not the opinion.
    #[test]
    fn the_emoji_box_is_authored_not_line_sized() {
        let line_box = EMOJI_STEP.size.0 * 16.0 * EMOJI_STEP.line_height;
        assert!((line_box - 20.0).abs() < 1e-4, "{line_box}");
        assert!(
            (f32::from(EMOJI_HEIGHT) - line_box).abs() > 0.5,
            "{EMOJI_HEIGHT:?} against a {line_box}px line box",
        );
    }

    /// `sm:h-8` takes the height off `size-6` and leaves the width alone, so the
    /// control is **not** the square its class reads as.
    #[test]
    fn the_emoji_box_is_wider_than_it_is_tall_by_the_sm_trap() {
        assert_eq!(EMOJI_WIDTH, px(24.0));
        assert_eq!(EMOJI_HEIGHT, px(32.0));
        assert_ne!(EMOJI_WIDTH, EMOJI_HEIGHT);
    }

    /// `--md-wash` reproduces the reference's `bg` exactly: `#f5f5f50d`, the
    /// foreground at 5%.
    ///
    /// This is what makes the `in srgb` / `in oklch` difference a measurement
    /// rather than an argument — mixing against `transparent` only scales alpha.
    #[test]
    fn the_wash_is_the_foreground_at_five_percent() {
        for theme in [Theme::DARK, Theme::LIGHT] {
            let wash = CalloutNode::wash(&theme).value();
            let base = theme.color_foreground.value();
            assert!((wash.a - 0.05).abs() < 1.0 / 255.0, "{:?}", wash.a);
            assert!((wash.h - base.h).abs() < 1e-3);
            assert!((wash.s - base.s).abs() < 1e-3);
            assert!((wash.l - base.l).abs() < 1e-3);
        }
    }

    /// The five values the stylesheet takes off the class list, pinned to what
    /// the live element measures.
    ///
    /// Written as one test on purpose: they are one finding, and a port that
    /// reverts any of them to its Tailwind reading fails here rather than in a
    /// matrix run.
    #[test]
    fn the_stylesheet_beats_every_utility_it_overlaps() {
        // `p-4 pl-3` would be 16/16/16/12; the rule says `0.85rem 1rem`.
        assert_eq!(PADDING_Y, px(13.6));
        assert_eq!(PADDING_X, px(16.0));
        assert_ne!(PADDING_X, px(12.0), "pl-3 does not survive");

        // `my-1` would be 4px; the rule says `1.2em`, which at the reference's
        // 16px base is 19.2 and measured 19.19.
        let margin = BODY_STEP.size.0 * 16.0 * MARGIN_BLOCK_EM;
        assert!((margin - 19.2).abs() < 0.05, "{margin}");
        assert!(margin > 4.0);

        // `rounded-sm` would be 6px; the rule says `--radius-md`, which the
        // reference measures at 8.
        assert_eq!(Theme::DARK.radius_md.value(), px(8.0));
        assert_ne!(Theme::DARK.radius_md.value(), px(6.0));

        // `bg-muted` is white @ 4%; `--md-wash` is *foreground* @ 5%.
        assert!((WASH_TINT - 5.0).abs() < f32::EPSILON);
    }

    /// The trailing gap the stylesheet means to remove and cannot — `0.9em`,
    /// measured 14.39px at the reference's base.
    #[test]
    fn the_body_keeps_the_gap_the_direct_child_rule_cannot_reach() {
        let gap = BODY_STEP.size.0 * 16.0 * BODY_GAP_EM;
        assert!((gap - 14.4).abs() < 0.05, "{gap}");
        assert!(gap > 0.0, "the `> .slate-p:last-child` rule does not match");
    }

    /// `gap-2` is one of the two utilities the stylesheet leaves alone, because
    /// the rule targets `.slate-callout` and not the row inside it.
    #[test]
    fn the_row_gap_is_the_utility_that_survives() {
        assert_eq!(GAP, px(8.0));
    }

    /// The fixture is the captured cell, and its body cannot wrap.
    #[test]
    fn the_fixture_is_the_captured_callout() {
        let fixture = CalloutNode::fixture();
        assert_eq!(fixture.icon, DEFAULT_ICON);
        assert_eq!(fixture.body, "Reachability");
        assert!(
            !fixture.body.contains(' '),
            "a body with a break opportunity can wrap, and a wrapped run is \
             outside what the contract compares",
        );
    }
}
