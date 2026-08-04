//! `loading-spinner` — a [`Spinner`] with a gap and an optional caption, and the
//! one compound of the three.
//!
//! The native half of `web/src/components/ui/loading-spinner.tsx`: an
//! `inline-flex` span holding a `<Spinner>` and, when `showLabel`, a muted
//! caption. Every value below came out of the app's own `tailwindcss` 4.3.0 with
//! the utility as a candidate — the method `native/MAPPING.md` fixes — and each
//! is confirmed against the captured reference. See
//! `native/mapping/loading-spinner.md` and `/tmp/p3-ref-loading-spinner.json`.
//!
//! # v1.9 reaches exactly one of this surface's three anchors
//!
//! The check is per anchor rather than per surface, and here the answer differs
//! within one snapshot:
//!
//! | anchor | animated? |
//! |---|---|
//! | `loading-spinner` | **no.** `transform` does not participate in layout, so the wrapper's own border box is unmoved by the glyph spinning inside it |
//! | `spinner` | **yes** — `bounds` travel 6.63px on a 16px box. See [`crate::primitives::spinner`] |
//! | `loading-spinner-label` | **no.** Nothing in the caption animates |
//!
//! The wrapper's immunity is a property of CSS rather than luck: a transformed
//! box keeps its layout box, so the flex line it sits on is laid out from the
//! untransformed 16px. Measured — the reference's wrapper is 138 × 18 at every
//! instant, and 138 = 16 + 6 + 116 with the spinner's *layout* box in it.
//!
//! The reference was nonetheless captured with the animation pinned at
//! `currentTime = 0`, because one of its three anchors needs it.
//!
//! # `ui-text-sm` **wins** here — the mirror of `label`'s trap
//!
//! `native/MAPPING.md` records that on `label` the primitive's `sm:text-sm/4`
//! beats a call site's `ui-text-sm`, so the rendered size is 14px and not the
//! 12px the class names. The caption here carries the same `ui-text-sm` and
//! **there is no `sm:` counterpart anywhere on it**, so the `@utility` stands.
//! Measured live: `font-size: 12px`, and the reference says `font.size 12`.
//!
//! Which is the point of the trap being stated as "measure, do not infer": the
//! same class resolves to two different numbers on two components, and only the
//! competing declaration decides which.
//!
//! # The caption's line box is preflight's, not a `leading-*`
//!
//! Nothing on the caption sets `line-height`. `ui-text-sm` compiles to
//! `font-size: var(--ui-text-sm)` and nothing else, so the box inherits
//! Tailwind preflight's `html, :host { line-height: 1.5 }` — measured live as
//! `line-height: 18px` against a 12px font, and the reference's
//! `font.line_height 18`. It is [`LINE_HEIGHT`] rather than a `TypeStep`
//! constant precisely because the two halves come from different places: the
//! size is a **sealed token** and the ratio is preflight's.

use gpui::{
    AnyElement, Div, FontWeight, ParentElement as _, Pixels, Rems, SharedString, Styled as _, div,
    px, relative,
};

use super::spinner::{self, Extent, Spinner};
use crate::anchor::{AnchorId, AnchorSink};
use crate::surfaces::rows::git_status_row::Breakpoint;
use crate::theme::{Theme, ui_sans_font};

/// The wrapper, and this surface's root anchor.
pub const ID_ROOT: &str = "loading-spinner";

/// The caption. Present only where the call site asks for one.
pub const ID_LABEL: &str = "loading-spinner-label";

/// Both boxes size to their content: the wrapper is `inline-flex` with no
/// authored width, and the caption is a non-growing flex item whose used width
/// is its run's max-content width.
///
/// `spinner` is deliberately **not** here: its box is authored by `size-4`.
pub const CONTENT_SIZED: [&str; 2] = [ID_ROOT, ID_LABEL];

/// Only the caption. The wrapper paints no text of its own, so it carries no
/// `font` — and `ANCHORS.md` v1.6 makes the declaration a refusal rather than a
/// delta on an anchor without one.
pub const LINE_SIZED: [&str; 1] = [ID_LABEL];

/// `gap-1.5` — `calc(var(--spacing) * 1.5)` at the stock `--spacing: 0.25rem`.
/// Measured live as `gap: 6px`, and the reference's caption sits at x = 22 = 16 + 6.
pub const GAP: Pixels = px(6.0);

/// `gap-1` — 4px, the `compact` shape.
pub const GAP_COMPACT: Pixels = px(4.0);

/// The caption's line-height ratio: Tailwind preflight's `line-height: 1.5`,
/// inherited unitless. See the module docs — nothing on the caption authors one.
pub const LINE_HEIGHT: f32 = 1.5;

/// The caption's weight. Nothing sets one, so preflight's `font-weight: inherit`
/// leaves the document default; the reference reports **400**.
pub const WEIGHT: FontWeight = FontWeight::NORMAL;

/// The default `label` prop, which is what a call site passing none gets.
pub const DEFAULT_LABEL: &str = "Loading";

/// The `props` bundle a call site passes.
///
/// Four live call sites across two files, and they differ in all three props —
/// which is why this is a vocabulary rather than three booleans: the label is a
/// *string the product paints*, and inventing one would put text on screen the
/// app never shows.
#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub enum CallSite {
    /// `<LoadingSpinner />` — `review-diff-tab.tsx`'s `<Suspense>` fallback
    /// around the lazy code view. No label shown, not compact.
    Fallback,
    /// `<LoadingSpinner label="Connecting" compact />` —
    /// `editor-status-actions.tsx`'s LSP status chip. Compact, and the label is
    /// passed but **not shown**: it reaches the glyph as its `aria-label`, which
    /// no field in the contract records.
    ConnectingChip,
    /// `<LoadingSpinner label="Connecting" showLabel compact />` — the same
    /// file's LSP menu row. The only compact shape that paints its caption.
    ConnectingRow,
    /// `<LoadingSpinner label="Loading branch diff" showLabel />` —
    /// `review-diff-tab.tsx` while a branch review's file summary is in flight.
    BranchDiff,
    /// `<LoadingSpinner label="Loading commit diff" showLabel />` — the same
    /// component scoped to one commit. **This is the captured cell**: it is the
    /// one a fresh `ReviewDiffTab` mount reaches from a click on a commit in the
    /// history panel.
    CommitDiff,
}

/// Every modelled call site, for `--help` and the closed-vocabulary test.
pub const ALL_CALL_SITES: [CallSite; 5] = [
    CallSite::Fallback,
    CallSite::ConnectingChip,
    CallSite::ConnectingRow,
    CallSite::BranchDiff,
    CallSite::CommitDiff,
];

impl CallSite {
    /// The word `--call-site` takes.
    #[must_use]
    pub fn name(self) -> &'static str {
        match self {
            Self::Fallback => "fallback",
            Self::ConnectingChip => "connecting-chip",
            Self::ConnectingRow => "connecting-row",
            Self::BranchDiff => "branch-diff",
            Self::CommitDiff => "commit-diff",
        }
    }

    /// The `label` prop. Painted only where [`CallSite::shows_label`].
    #[must_use]
    pub fn label(self) -> &'static str {
        match self {
            Self::Fallback => DEFAULT_LABEL,
            Self::ConnectingChip | Self::ConnectingRow => "Connecting",
            Self::BranchDiff => "Loading branch diff",
            Self::CommitDiff => "Loading commit diff",
        }
    }

    /// The `showLabel` prop.
    #[must_use]
    pub fn shows_label(self) -> bool {
        matches!(
            self,
            Self::ConnectingRow | Self::BranchDiff | Self::CommitDiff
        )
    }

    /// The `compact` prop, which moves the gap **and** the glyph.
    #[must_use]
    pub fn compact(self) -> bool {
        matches!(self, Self::ConnectingChip | Self::ConnectingRow)
    }
}

/// One `<LoadingSpinner>`.
#[derive(Clone, Copy, Debug, PartialEq)]
pub struct LoadingSpinner {
    /// The props bundle a call site passes.
    pub call_site: CallSite,
    /// §8.3's `empty`: the caption slot with nothing in it.
    ///
    /// A `<LoadingSpinner>`'s own content **is** its label — the glyph is its
    /// icon, and every call site that shows nothing else is icon-only by
    /// design. So `empty` drops the caption, which drops an anchor, and it is
    /// the same picture [`CallSite::Fallback`] reaches from the other direction.
    /// Unlike `spinner`'s `empty` this one is live: it is what the `<Suspense>`
    /// fallback and the status chip render.
    pub empty: bool,
    /// Which side of `sm:` the viewport is on. Passed through to the glyph;
    /// nothing on this component's own class list carries a `sm:` variant.
    pub breakpoint: Breakpoint,
}

impl LoadingSpinner {
    /// The captured cell: a freshly-mounted commit `ReviewDiffTab` at a 1714px
    /// viewport — wrapper `138 × 18`, glyph `16 × 16` at `y 1`, caption
    /// `116 × 18` at `x 22` reading `Loading commit diff`, `fg #a4a4a4ff`,
    /// `CalSansUI` 12/18 at weight 400.
    #[must_use]
    pub fn fixture() -> Self {
        Self {
            call_site: CallSite::CommitDiff,
            empty: false,
            breakpoint: Breakpoint::Sm,
        }
    }

    /// Whether this cell paints a caption at all.
    #[must_use]
    pub fn shows_label(self) -> bool {
        !self.empty && self.call_site.shows_label()
    }

    /// The string the caption paints, or `None` where there is none.
    #[must_use]
    pub fn label(self) -> Option<SharedString> {
        self.shows_label()
            .then(|| SharedString::new_static(self.call_site.label()))
    }

    /// The gap between the glyph and the caption.
    ///
    /// Present whether or not there is a caption — CSS `gap` needs two items to
    /// show, so on an icon-only cell it is inert, exactly as `label`'s is.
    #[must_use]
    pub fn gap(self) -> Pixels {
        if self.call_site.compact() {
            GAP_COMPACT
        } else {
            GAP
        }
    }

    /// The glyph this cell holds, as its own component.
    ///
    /// The composition is the React one: `LoadingSpinner` renders a `<Spinner>`
    /// and merges a `size-*` onto it, so the native side reads the size off
    /// [`spinner::CallSite`] rather than restating a number here.
    #[must_use]
    pub fn spinner(self) -> Spinner {
        Spinner {
            extent: Extent::Class(if self.call_site.compact() {
                spinner::CallSite::LoadingSpinnerCompact
            } else {
                spinner::CallSite::LoadingSpinner
            }),
            breakpoint: self.breakpoint,
        }
    }

    /// The wrapper's box.
    ///
    /// `inline-flex` becomes `.flex()` for `badge`'s reason: gpui has no inline
    /// flow, and the live computed `display` is **`flex`** anyway — measured,
    /// because CSS blockifies a flex item's display and every live call site
    /// puts this span in a flex container.
    ///
    /// No background, no radius, no border. The reference agrees on all three.
    fn shell(self) -> Div {
        div().flex().items_center().gap(self.gap())
    }

    /// The caption's box.
    ///
    /// A block-level flex item, which is what the live `<span>` computes to —
    /// `display: block` measured, CSS having blockified it. The family is named
    /// rather than inherited for `git_status_row`'s reason: `ANCHORS.md` v1.2
    /// makes `font.family` the *declared* first family, and an inherited style
    /// reports macOS's `.SystemUIFont`, a string the DOM never produces. The
    /// reference says `CalSansUI`.
    ///
    /// The size is the sealed `--ui-text-sm` token rather than a literal, so a
    /// project that moves `--app-ui-scale` moves this with it.
    fn caption(theme: &Theme) -> Div {
        div()
            .font(ui_sans_font(theme))
            .text_size(Rems::from(theme.ui_text_sm))
            .line_height(relative(LINE_HEIGHT))
            .font_weight(WEIGHT)
            .text_color(theme.color_muted_foreground)
    }

    /// The element and its anchors: the wrapper, the glyph, and the caption
    /// where there is one.
    ///
    /// The caption is a **sibling** of the glyph rather than a child of a second
    /// wrapper — the DOM is `[svg, span]` under one span, and an extra flex row
    /// would give the root anchor a box that is not the one the reference
    /// measures.
    pub fn render(self, theme: &Theme, anchors: &dyn AnchorSink) -> AnyElement {
        let mut shell = self.shell().child(self.spinner().render(anchors));

        if let Some(text) = self.label() {
            shell = shell.child(anchors.boxed_text(
                AnchorId::from(ID_LABEL).content_sized().line_sized(),
                Self::caption(theme),
                text,
            ));
        }

        anchors.boxed(AnchorId::from(ID_ROOT).content_sized(), shell)
    }
}

#[cfg(test)]
mod tests {
    use super::{
        ALL_CALL_SITES, CallSite, DEFAULT_LABEL, GAP, GAP_COMPACT, ID_LABEL, ID_ROOT, LINE_HEIGHT,
        LoadingSpinner, WEIGHT,
    };
    use crate::primitives::spinner;
    use crate::theme::{Appearance, Theme};
    use gpui::{FontWeight, Rems, px};

    /// Both declarations, and on the anchors they are true of. `spinner` is in
    /// neither list — its box is authored — which is the control that stops the
    /// assertion passing whatever the lists held.
    #[test]
    fn the_wrapper_and_the_caption_are_content_sized_and_only_the_caption_is_line_sized() {
        assert_eq!(super::CONTENT_SIZED, [ID_ROOT, ID_LABEL]);
        assert_eq!(super::LINE_SIZED, [ID_LABEL]);
        assert!(!super::CONTENT_SIZED.contains(&spinner::ID_SPINNER));
        assert!(!super::LINE_SIZED.contains(&spinner::ID_SPINNER));
    }

    /// The reference's arithmetic, from the constants: `16 + 6 = 22` is the
    /// caption's x, and `22 + 116 = 138` is the wrapper's width.
    #[test]
    fn the_gap_accounts_for_the_captured_geometry() {
        let fixture = LoadingSpinner::fixture();
        assert_eq!(fixture.gap(), GAP);

        let glyph = f32::from(fixture.spinner().size());
        assert!((glyph - 16.0).abs() < f32::EPSILON);
        let caption_x = glyph + f32::from(GAP);
        assert!((caption_x - 22.0).abs() < f32::EPSILON, "got {caption_x}");

        let caption_advance = 115.99_f32;
        // `ANCHORS.md` v1.5: the caption's box is `ceil` of its run on the
        // native side, and the wrapper is the sum.
        let wrapper = caption_x + caption_advance.ceil();
        assert!((wrapper - 138.0).abs() < f32::EPSILON, "got {wrapper}");
    }

    /// `compact` moves the gap **and** the glyph together — it is one prop with
    /// two consequences, and a port that carried only the gap across would be
    /// 4px wrong on the chip.
    #[test]
    fn compact_moves_both_the_gap_and_the_glyph() {
        for call_site in ALL_CALL_SITES {
            let cell = LoadingSpinner {
                call_site,
                ..LoadingSpinner::fixture()
            };
            let (gap, glyph) = if call_site.compact() {
                (GAP_COMPACT, px(12.0))
            } else {
                (GAP, px(16.0))
            };
            assert_eq!(cell.gap(), gap, "{}", call_site.name());
            assert_eq!(cell.spinner().size(), glyph, "{}", call_site.name());
        }
        // And the two shapes really are different numbers.
        assert_ne!(GAP, GAP_COMPACT);
    }

    /// Three of the five call sites paint a caption, and `empty` drops it from
    /// **every** one of them — landing on the same picture
    /// [`CallSite::Fallback`] reaches from the other direction.
    #[test]
    fn empty_drops_the_caption_from_every_call_site() {
        for call_site in ALL_CALL_SITES {
            let shown = LoadingSpinner {
                call_site,
                empty: false,
                ..LoadingSpinner::fixture()
            };
            assert_eq!(shown.shows_label(), call_site.shows_label());
            assert_eq!(shown.label().is_some(), call_site.shows_label());

            let empty = LoadingSpinner {
                empty: true,
                ..shown
            };
            assert!(!empty.shows_label(), "{}", call_site.name());
            assert_eq!(empty.label(), None, "{}", call_site.name());
        }
        assert!(!CallSite::Fallback.shows_label());
        // The claim is not vacuous: three of the five do show one.
        assert_eq!(
            ALL_CALL_SITES
                .iter()
                .filter(|site| site.shows_label())
                .count(),
            3,
        );
    }

    /// The caption's type step: the sealed `--ui-text-sm` token at 12px on
    /// preflight's 1.5 ratio, which is the reference's 12/18. `ui-text-sm` wins
    /// here where it loses on `label` — see the module docs.
    #[test]
    fn the_caption_is_the_token_size_on_preflights_line_height() {
        for appearance in [Appearance::Light, Appearance::Dark] {
            let theme = Theme::for_appearance(appearance);
            let size = Rems::from(theme.ui_text_sm);
            assert!(
                (size.0 * 16.0 - 12.0).abs() < 1e-4,
                "{appearance:?}: got {size:?}",
            );
            let line_box = size.0 * 16.0 * LINE_HEIGHT;
            assert!((line_box - 18.0).abs() < 1e-4, "got {line_box}");
        }
        assert_eq!(WEIGHT, FontWeight::NORMAL);
    }

    /// The fixture is the captured cell, and the default label is the prop's own
    /// default rather than a string this file invented.
    #[test]
    fn the_fixture_is_the_captured_commit_diff_cell() {
        let fixture = LoadingSpinner::fixture();
        assert_eq!(fixture.call_site, CallSite::CommitDiff);
        assert!(!fixture.empty);
        assert_eq!(fixture.label().expect("shown"), "Loading commit diff");
        assert_eq!(CallSite::Fallback.label(), DEFAULT_LABEL);
    }

    /// The vocabulary is closed and its words are unique.
    #[test]
    fn the_call_site_vocabulary_is_closed() {
        let mut names: Vec<_> = ALL_CALL_SITES.iter().map(|c| c.name()).collect();
        names.sort_unstable();
        assert_eq!(
            names,
            [
                "branch-diff",
                "commit-diff",
                "connecting-chip",
                "connecting-row",
                "fallback",
            ],
        );
    }
}
