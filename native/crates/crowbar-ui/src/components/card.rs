//! `card` — a slotted container, and the P3.10 surface that **must not declare
//! its anchor set**.
//!
//! The native half of `web/src/components/ui/card.tsx`, which exports four
//! slots: `Card`, `CardHeader`, `CardTitle` and `CardPanel` (re-exported as
//! `CardContent`). See `native/mapping/card.md`.
//!
//! # v1.8: the anchor set here is a property of the **cell**, not of the surface
//!
//! `ANCHORS.md` v1.8 permits a surface to declare its anchor set "only when that
//! set is a property of the surface rather than of the cell". A `Card` fails
//! that test by construction: every slot is optional and the call site chooses
//! which to fill, so `card-header`, `card-title` and `card-panel` are each
//! present or absent per cell. A fixed list would be wrong in most of them, and
//! v1.8's loud-missing rule would then reject honest captures.
//!
//! So this surface declares nothing on the reference side, exactly as
//! `git-status-row` declares nothing for the same reason. What follows from that
//! is recorded in the mapping rather than worked around here: an
//! `ErrorBoundary` capture would also pick up the `button` anchor of the Try
//! again control, which is another surface's. Resolving it needs either a
//! call-site rename (`git-row-badge`'s route) or an `oracleSurfaceScope` entry,
//! and both are the orchestrator's to choose — this port does not fabricate one.
//!
//! # There is no reference: the Card renders only from a caught render throw
//!
//! `[data-slot=card]` has **zero** live instances, and the single importer is
//! `components/error-boundary.tsx`, whose fallback needs a render-phase throw in
//! a boundary that was given no `fallback` prop. Three such boundaries exist
//! (`ide-shell.tsx` twice, `sidebar-carousel.tsx` once) and all three wrap
//! ordinary app subtrees, so reaching one means *introducing a defect*. The one
//! documented render throw in the tree — `mermaid-diagram.tsx`'s — is caught by
//! `markdown-editor-pane.tsx`'s boundary, which **does** pass a fallback and so
//! never reaches a Card.
//!
//! Every value below therefore comes from the utilities, resolved through the
//! app's own compiled `tailwindcss` 4.3.0 and read back off a probe element in
//! the live document. **No reference JSON was fabricated.** `separator` and
//! `skeleton` are the precedent.
//!
//! # The `in-[…]` variants are the real content, and one of them beats the call site
//!
//! Three of the four slots carry a variant keyed on what the *card* contains:
//!
//! ```text
//! CardHeader  in-[[data-slot=card]:has(>[data-slot=card-panel])]:pb-4
//! CardPanel   in-[[data-slot=card]:has(>[data-slot=card-header]:not(.border-b))]:pt-0
//! CardPanel   in-[[data-slot=card]:has(>[data-slot=card-footer]:not(.border-t))]:pb-0
//! ```
//!
//! They are why [`Card`] is modelled as a set of slots rather than as one box:
//! the header's bottom padding and the panel's top padding are **functions of
//! which siblings exist**, and a port that hard-coded `p-6` would be 8px out on
//! two edges of the only shape the app builds.
//!
//! The header's is also the `sm:`-trap in a third guise. `error-boundary.tsx`
//! writes `pb-2` on the header, and the probe measures **16px** — `in-[…]:pb-4`,
//! not the call site's 8. Different tailwind-merge modifiers keep both classes
//! and Tailwind emits the variant later, so the call site loses. `badge` hit this
//! with `sm:`, `label` hit it in reverse, and this is the same mechanism with a
//! third kind of prefix.
//!
//! By contrast `CardTitle` is where the call site **does** win: its `text-sm`
//! and the primitive's `text-lg` are the same tailwind-merge group, so `cn(…)`
//! drops the primitive's and the probe reads 14px. The two facts sit one slot
//! apart, which is why neither can be reasoned about from the class list.
//!
//! # gpui has no CSS grid
//!
//! `CardHeader` is `grid auto-rows-min grid-rows-[auto_auto]` with a
//! `has-data-[slot=card-action]:grid-cols-[1fr_auto]` second column. **No live
//! call site passes a `card-action`**, so every real header is a single column of
//! auto-height rows with a `gap-1.5` between them — which is a flex column with a
//! gap, and that is what this renders. The two-column arrangement is *not*
//! ported and is recorded as absent rather than approximated; see
//! [`Slots::action`].

use gpui::{
    AnyElement, Div, FontWeight, IntoElement as _, ParentElement as _, Pixels, Rems, SharedString,
    Styled as _, div, px, relative,
};

use super::anchor::{AnchorId, AnchorSink};
use super::badge::TypeStep;
use crate::theme::{Color, Theme, ui_sans_font};

/// The card's own box — the surface root.
pub const ID_CARD: &str = "card";

/// `CardHeader`.
pub const ID_HEADER: &str = "card-header";

/// `CardTitle`.
pub const ID_TITLE: &str = "card-title";

/// `CardPanel`, exported as `CardContent`.
pub const ID_PANEL: &str = "card-panel";

/// **Nothing.** The card is `w-full` under a `max-w-sm`, and every slot is a
/// stretched block or grid item.
pub const CONTENT_SIZED: [&str; 0] = [];

/// The title alone: it authors no height, so its box *is* its line box.
///
/// `ANCHORS.md` v1.6's test is "derived from the line box", and `leading-none`
/// makes that ratio exactly 1 — the probe reads `font-size: 14px`,
/// `line-height: 14px`, `height: 14`.
pub const LINE_SIZED: [&str; 1] = [ID_TITLE];

/// `p-6` on the header and the panel — measured `24px`.
pub const PADDING: Pixels = px(24.0);

/// `in-[…]:pb-4` on a header whose card also holds a panel — measured `16px`,
/// and it **beats** `error-boundary.tsx`'s own `pb-2`. See the module docs.
pub const HEADER_PADDING_BOTTOM_WITH_PANEL: Pixels = px(16.0);

/// `gap-1.5` between the header's rows — measured `6px`.
pub const HEADER_GAP: Pixels = px(6.0);

/// `max-w-sm` — Tailwind's stock `--container-sm`, `24rem`. Measured `384px`.
///
/// A plain constant rather than a token: `--container-*` is Tailwind's own
/// scale and is not one of the 180 names `theme.css` declares, so there is
/// nothing in the sealed table to read it from.
pub const MAX_WIDTH: Pixels = px(384.0);

/// `text-lg` — the primitive's own step, `18px` on `calc(1.75 / 1.125)`.
///
/// **Dead at the only live call site**, which writes `text-sm` in the same
/// tailwind-merge group. Kept because [`CallSite::None`] is a real, if unused,
/// rendering and because the contrast with the header's `pb` is the finding.
pub const TITLE_STEP: TypeStep = TypeStep {
    size: Rems(1.125),
    line_height: 1.0,
};

/// `text-sm` over the primitive's `text-lg` — `14px`, and `leading-none` keeps
/// the ratio at 1, so the probe reads a `14px` line box.
pub const TITLE_STEP_SM: TypeStep = TypeStep {
    size: Rems(0.875),
    line_height: 1.0,
};

/// `font-semibold` on the title — the probe reads `600`.
pub const TITLE_WEIGHT: FontWeight = FontWeight::SEMIBOLD;

/// `bg-destructive/10`'s mix percentage.
pub const TINT_BACKGROUND: f32 = 10.0;

/// `border-destructive/20`'s mix percentage.
pub const TINT_BORDER: f32 = 20.0;

/// The `className` bundle a call site merges over the primitive's own.
///
/// One arm per live call site, plus the unmerged primitive — `button`'s
/// `RadiusClass` and `label`'s `CallSite` established that a call site's
/// className is a legitimate parameter, and it names the **class**, never the
/// number.
#[derive(Clone, Copy, Debug, Default, PartialEq, Eq)]
pub enum CallSite {
    /// No className: the primitive's `bg-card`, `border` and `text-lg` stand.
    ///
    /// **No live call site does this**, so it has no reference even in
    /// principle. Rendered for `resizable`'s reason about the grip.
    None,
    /// `error-boundary.tsx`: `w-full max-w-sm border-destructive/20
    /// bg-destructive/10` on the card, `pb-2` on the header (dead — see the
    /// module docs), `text-sm text-destructive` on the title.
    ///
    /// **The only shape the app can build**, and it is behind a caught render
    /// throw.
    #[default]
    ErrorBoundary,
}

/// Every modelled call site, for `--help` and the closed-vocabulary test.
pub const ALL_CALL_SITES: [CallSite; 2] = [CallSite::None, CallSite::ErrorBoundary];

impl CallSite {
    /// The word `--call-site` takes.
    #[must_use]
    pub fn name(self) -> &'static str {
        match self {
            Self::None => "none",
            Self::ErrorBoundary => "error-boundary",
        }
    }

    /// The card's background.
    #[must_use]
    pub fn background(self, theme: &Theme) -> Color {
        match self {
            Self::None => theme.card,
            Self::ErrorBoundary => theme.destructive.mix(TINT_BACKGROUND, Color::TRANSPARENT),
        }
    }

    /// The card's border colour. A **painted 1px** either way — the class list
    /// carries a bare `border`, which is `badge`'s and `button`'s trap and the
    /// opposite of `kbd`'s.
    #[must_use]
    pub fn border(self, theme: &Theme) -> Color {
        match self {
            Self::None => theme.border,
            Self::ErrorBoundary => theme.destructive.mix(TINT_BORDER, Color::TRANSPARENT),
        }
    }

    /// The title's colour and type step.
    #[must_use]
    pub fn title(self, theme: &Theme) -> (Color, TypeStep) {
        match self {
            Self::None => (theme.card_foreground, TITLE_STEP),
            Self::ErrorBoundary => (theme.destructive, TITLE_STEP_SM),
        }
    }
}

/// Which slots a cell fills, as a **closed vocabulary**.
///
/// The anchor set *is* this value — which is exactly why the surface declares no
/// set on the reference side (v1.8, see the module docs).
///
/// An enum rather than a struct of flags, and that is not only clippy's
/// `struct_excessive_bools`: only some combinations are pictures the app can
/// build, and a free-form set would let a cell ask for the ones it cannot. In
/// particular there is **no `action` arm**, so the two-column header gpui cannot
/// lay out is unrepresentable rather than merely discouraged.
#[derive(Clone, Copy, Debug, Default, PartialEq, Eq)]
pub enum Slots {
    /// Header + title + panel — `error-boundary.tsx`'s shape, and the only one
    /// the app builds.
    #[default]
    HeaderAndPanel,
    /// A header with no panel: where `in-[…]:pb-4` stops matching and the header
    /// keeps its full `p-6`.
    HeaderOnly,
    /// A panel with no header: where `in-[…]:pt-0` stops matching.
    PanelOnly,
    /// Header + title + panel + a footer: where `in-[…]:pb-0` starts matching
    /// and the panel gives up its bottom padding.
    ///
    /// `card.tsx` exports no `CardFooter`, so the footer itself is never drawn —
    /// only its **effect on the panel**, which is what the CSS keys on.
    Footed,
    /// §8.3's `empty`: a `<Card>` with no children at all, which paints its
    /// border and its tint and nothing else.
    Empty,
}

/// Every arrangement, for `--help` and the closed-vocabulary test.
pub const ALL_SLOTS: [Slots; 5] = [
    Slots::Empty,
    Slots::Footed,
    Slots::HeaderAndPanel,
    Slots::HeaderOnly,
    Slots::PanelOnly,
];

impl Slots {
    /// The word `--slots` takes.
    #[must_use]
    pub const fn name(self) -> &'static str {
        match self {
            Self::HeaderAndPanel => "header-and-panel",
            Self::HeaderOnly => "header-only",
            Self::PanelOnly => "panel-only",
            Self::Footed => "footed",
            Self::Empty => "empty",
        }
    }

    /// Whether a `CardHeader` is rendered.
    #[must_use]
    pub const fn header(self) -> bool {
        matches!(self, Self::HeaderAndPanel | Self::HeaderOnly | Self::Footed)
    }

    /// Whether a `CardTitle` is rendered. Only ever inside a header.
    #[must_use]
    pub const fn title(self) -> bool {
        self.header()
    }

    /// Whether a `CardPanel` is rendered.
    #[must_use]
    pub const fn panel(self) -> bool {
        matches!(self, Self::HeaderAndPanel | Self::PanelOnly | Self::Footed)
    }

    /// Whether a `card-footer` is present.
    ///
    /// Never *drawn* — see [`Slots::Footed`] — but load-bearing for the panel's
    /// bottom padding, which is what the CSS keys on.
    #[must_use]
    pub const fn footer(self) -> bool {
        matches!(self, Self::Footed)
    }

    /// Whether a `card-action` is present. **Always `false`**: no live call site
    /// passes one, gpui has no CSS grid, and this vocabulary has no arm for it,
    /// so the picture that would be measured wrong cannot be asked for.
    #[must_use]
    pub const fn action(self) -> bool {
        false
    }

    /// `in-[[data-slot=card]:has(>[data-slot=card-panel])]:pb-4`.
    #[must_use]
    pub fn header_padding_bottom(self) -> Pixels {
        if self.panel() {
            HEADER_PADDING_BOTTOM_WITH_PANEL
        } else {
            PADDING
        }
    }

    /// `in-[[data-slot=card]:has(>[data-slot=card-header]:not(.border-b))]:pt-0`.
    #[must_use]
    pub fn panel_padding_top(self) -> Pixels {
        if self.header() { px(0.0) } else { PADDING }
    }

    /// `in-[[data-slot=card]:has(>[data-slot=card-footer]:not(.border-t))]:pb-0`.
    #[must_use]
    pub fn panel_padding_bottom(self) -> Pixels {
        if self.footer() { px(0.0) } else { PADDING }
    }
}

/// One `<Card>`.
#[derive(Clone, Debug, PartialEq)]
pub struct Card {
    /// The className bundle merged over the primitive's.
    pub call_site: CallSite,
    /// Which slots this cell fills.
    pub slots: Slots,
    /// The title's string.
    pub title: SharedString,
}

impl Card {
    /// `error-boundary.tsx`'s card: header + title + panel, the destructive tint,
    /// and the boundary's own heading.
    ///
    /// **The title is unbreakable at the width the card has.** `max-w-sm` gives a
    /// 384px card and a 334px title box, and `Something went wrong` shapes well
    /// inside it — but the string is fixed here rather than taken from a
    /// `--content` axis for the usual reason: a run that wraps is outside what
    /// the contract can compare.
    #[must_use]
    pub fn fixture() -> Self {
        Self {
            call_site: CallSite::ErrorBoundary,
            slots: Slots::default(),
            title: SharedString::new_static("Something went wrong"),
        }
    }

    /// The card's own box.
    fn shell(&self, theme: &Theme) -> Div {
        div()
            .font(ui_sans_font(theme))
            .relative()
            .flex()
            .flex_col()
            .w_full()
            .max_w(MAX_WIDTH)
            .rounded(theme.radius_2xl.value())
            .border_1()
            .border_color(self.call_site.border(theme))
            .bg(self.call_site.background(theme))
            .text_color(theme.card_foreground)
    }

    /// `CardHeader` — a single-column grid, which is a flex column with a gap.
    fn header_box(&self) -> Div {
        div()
            .flex()
            .flex_col()
            .items_start()
            .gap(HEADER_GAP)
            .pt(PADDING)
            .px(PADDING)
            .pb(self.slots.header_padding_bottom())
    }

    /// `CardTitle`.
    fn title_box(&self, theme: &Theme) -> Div {
        let (color, step) = self.call_site.title(theme);
        div()
            .text_size(step.size)
            .line_height(relative(step.line_height))
            .font_weight(TITLE_WEIGHT)
            .text_color(color)
    }

    /// `CardPanel`.
    fn panel_box(&self) -> Div {
        div()
            .flex_1()
            .px(PADDING)
            .pt(self.slots.panel_padding_top())
            .pb(self.slots.panel_padding_bottom())
    }

    /// The element and the anchors this cell's slots produce.
    pub fn render(&self, theme: &Theme, anchors: &dyn AnchorSink) -> AnyElement {
        let mut element = self.shell(theme);

        if self.slots.header() {
            let mut header = self.header_box();
            if self.slots.title() {
                header = header.child(anchors.boxed_text(
                    AnchorId::from(ID_TITLE).line_sized(),
                    self.title_box(theme),
                    self.title.clone(),
                ));
            }
            element = element.child(anchors.boxed(ID_HEADER.into(), header));
        }

        if self.slots.panel() {
            element = element.child(anchors.boxed(ID_PANEL.into(), self.panel_box()));
        }

        anchors.root(ID_CARD.into(), element).into_any_element()
    }
}

#[cfg(test)]
mod tests {
    use super::{
        ALL_CALL_SITES, ALL_SLOTS, CONTENT_SIZED, CallSite, Card, HEADER_GAP,
        HEADER_PADDING_BOTTOM_WITH_PANEL, ID_TITLE, LINE_SIZED, MAX_WIDTH, PADDING, Slots,
        TINT_BACKGROUND, TINT_BORDER, TITLE_STEP, TITLE_STEP_SM, TITLE_WEIGHT,
    };
    use crate::theme::{Color, Theme};
    use gpui::{FontWeight, px};

    /// The title is the only line-sized anchor, and nothing is content-sized.
    #[test]
    fn only_the_title_is_line_sized() {
        assert!(CONTENT_SIZED.is_empty());
        assert_eq!(LINE_SIZED, [ID_TITLE]);
    }

    /// **The `in-[…]` variant beats the call site's `pb-2`** — the probe reads 16,
    /// not 8, and a port that took the call site's class would be 8px out.
    #[test]
    fn the_headers_bottom_padding_is_the_variants_and_not_the_call_sites() {
        let with_panel = Slots::default();
        assert!(with_panel.panel());
        assert_eq!(
            with_panel.header_padding_bottom(),
            HEADER_PADDING_BOTTOM_WITH_PANEL,
        );
        assert_eq!(with_panel.header_padding_bottom(), px(16.0));
        // Not `pb-2`, which is what `error-boundary.tsx` actually writes.
        assert_ne!(with_panel.header_padding_bottom(), px(8.0));

        // Without a panel the variant does not match and `p-6` stands.
        assert_eq!(Slots::HeaderOnly.header_padding_bottom(), PADDING);
    }

    /// The panel's two edges are functions of its siblings, exactly as the CSS
    /// states them.
    #[test]
    fn the_panels_padding_follows_the_slots_around_it() {
        let both = Slots::default();
        assert_eq!(both.panel_padding_top(), px(0.0), "a header is present");
        assert_eq!(both.panel_padding_bottom(), PADDING, "no footer is");

        assert_eq!(Slots::PanelOnly.panel_padding_top(), PADDING);
        assert_eq!(Slots::Footed.panel_padding_bottom(), px(0.0));
    }

    /// The two-column header is **not ported**, and no cell can ask for it.
    #[test]
    fn no_cell_can_request_the_ungridded_card_action() {
        for slots in ALL_SLOTS {
            assert!(!slots.action(), "{slots:?}");
        }
        // ...and the vocabulary has no arm that could turn it on: the words are
        // exactly these five, so the two-column header is unrepresentable.
        let mut names: Vec<_> = ALL_SLOTS.iter().map(|s| s.name()).collect();
        names.sort_unstable();
        assert_eq!(
            names,
            [
                "empty",
                "footed",
                "header-and-panel",
                "header-only",
                "panel-only"
            ],
        );
    }

    /// **The call site wins on the title and loses on the header** — the two
    /// facts one slot apart that make this component unreadable from its classes.
    #[test]
    fn the_call_site_takes_the_title_step_but_not_the_header_padding() {
        let theme = Theme::DARK;
        let (color, step) = CallSite::ErrorBoundary.title(&theme);
        assert_eq!(step.size, TITLE_STEP_SM.size, "text-sm beats text-lg");
        assert_ne!(step.size, TITLE_STEP.size);
        assert_eq!(color, theme.destructive);

        // ...while the header's own `pb-2` does not beat `in-[…]:pb-4`.
        assert_eq!(Slots::default().header_padding_bottom(), px(16.0));
    }

    /// `leading-none` makes both steps ratio 1, so a title's box is its font
    /// size — which is what `line_sized` claims.
    #[test]
    fn leading_none_makes_the_title_box_its_own_font_size() {
        for step in [TITLE_STEP, TITLE_STEP_SM] {
            assert!((step.line_height - 1.0).abs() < f32::EPSILON);
        }
        assert!((TITLE_STEP_SM.size.0 * 16.0 - 14.0).abs() < 1e-4);
        assert!((TITLE_STEP.size.0 * 16.0 - 18.0).abs() < 1e-4);
    }

    /// The tints are derived from the sealed `destructive`, never minted.
    #[test]
    fn the_destructive_tints_come_off_the_token() {
        for theme in [Theme::DARK, Theme::LIGHT] {
            let bg = CallSite::ErrorBoundary.background(&theme);
            let border = CallSite::ErrorBoundary.border(&theme);
            assert_eq!(
                bg,
                theme.destructive.mix(TINT_BACKGROUND, Color::TRANSPARENT)
            );
            assert_eq!(
                border,
                theme.destructive.mix(TINT_BORDER, Color::TRANSPARENT)
            );
            assert!((bg.value().a - 0.10).abs() < 1.0 / 255.0);
            assert!((border.value().a - 0.20).abs() < 1.0 / 255.0);
            // The unmerged primitive is a different picture entirely.
            assert_eq!(CallSite::None.background(&theme), theme.card);
            assert_eq!(CallSite::None.border(&theme), theme.border);
        }
    }

    /// The probe's geometry.
    #[test]
    fn the_box_values_are_the_probed_ones() {
        assert_eq!(MAX_WIDTH, px(384.0));
        assert_eq!(PADDING, px(24.0));
        assert_eq!(HEADER_GAP, px(6.0));
        assert_eq!(TITLE_WEIGHT, FontWeight::SEMIBOLD);
        assert_eq!(Theme::DARK.radius_2xl.value(), px(18.0));
    }

    /// The vocabulary is closed and its words are unique.
    #[test]
    fn the_call_site_vocabulary_is_closed() {
        let mut names: Vec<_> = ALL_CALL_SITES.iter().map(|c| c.name()).collect();
        names.sort_unstable();
        assert_eq!(names, ["error-boundary", "none"]);
        assert_eq!(Card::fixture().call_site, CallSite::ErrorBoundary);
    }
}
