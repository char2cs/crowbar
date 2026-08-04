//! `label` — a text run with a gap, and the component that caught the `sm:`
//! trap **pointing the other way**.
//!
//! The native half of `web/src/components/ui/label.tsx`, a `useRender` over a
//! `<label>` with one class list and no variants. Every value below came out of
//! the app's own `tailwindcss` 4.3.0 with the utility as a candidate — the
//! method `native/MAPPING.md` fixes — and each is confirmed against the captured
//! reference. See `native/mapping/label.md`.
//!
//! # The trap: here it is the **call site's** class that is dead
//!
//! `native/MAPPING.md` records that a call site's unprefixed class can be dead
//! above 640px, because tailwind-merge keeps both and Tailwind emits the `sm:`
//! variant later. On this component the same mechanism fires **in reverse**, and
//! it is worth writing down because the conclusion is the opposite one.
//!
//! The primitive carries `text-base/4.5 sm:text-sm/4`. Twelve of the live labels
//! are settings rows carrying the call site's own `ui-text-sm`, which resolves
//! to `font-size: var(--ui-text-sm)`. Measured live:
//!
//! ```text
//! --ui-text-sm    calc(0.75rem * 1)   = 12px
//! --ui-text-base  calc(0.875rem * 1)  = 14px
//! rendered font-size on ALL twelve    = 14px, line-height 16px
//! ```
//!
//! So `ui-text-sm` **loses**: the primitive's `sm:text-sm/4` is emitted in a
//! later layer than the `@utility`, and wins at every real window width. A port
//! that had read the call site and concluded "12px" would have been wrong by
//! 2px on every settings row — and the class that misleads is the *call site's*,
//! not the primitive's. The reference settles it: `font.size` **14**,
//! `font.line_height` **16**.
//!
//! # `content_sized` **and** `line_sized`, and why the second costs nothing here
//!
//! The box is `inline-flex` with no authored width or height, so both hold:
//! its width is the run's max-content width, and its height *is* the run's line
//! box. `ANCHORS.md` v1.6 warns that declaring `line_sized` wrongly manufactures
//! a delta, so the test is whether the height is **derived from** the line box
//! rather than authored — unlike `badge`, which authors `h-5` and is therefore
//! not line-sized.
//!
//! On the captured cell the declaration is numerically free: `bounds.h` is 16
//! and `font.line_height` is 16, so both comparisons ask the same question.
//! It is declared anyway because it is the **true** shape, and it is what keeps
//! the anchor honest if the type step ever moves off an integer line box.

use gpui::{
    AnyElement, Div, FontWeight, Pixels, Rems, SharedString, Styled as _, div, px, relative,
};

use crate::anchor::{AnchorId, AnchorSink};
use super::badge::TypeStep;
use crate::surfaces::rows::git_status_row::Breakpoint;
use crate::theme::{Theme, ui_sans_font};

/// The single anchor this surface carries.
pub const ID_LABEL: &str = "label";

/// The box is `inline-flex` with no authored width: its used width is the run's
/// max-content width, which is exactly what `ANCHORS.md` v1.5 is about.
pub const CONTENT_SIZED: [&str; 1] = [ID_LABEL];

/// The box authors no height either, so its height *is* its line box — see the
/// module docs for why that is a different question from "it paints text".
pub const LINE_SIZED: [&str; 1] = [ID_LABEL];

/// `gap-2` — `calc(var(--spacing) * 2)` at the stock `--spacing: 0.25rem`.
///
/// Measured live as `gap: 8px`. It separates the label's text from an icon or
/// accessory a call site puts beside it; with a single text child it is inert,
/// which is the case the reference was captured in.
pub const GAP: Pixels = px(8.0);

/// `font-medium`.
pub const WEIGHT: FontWeight = FontWeight::MEDIUM;

/// `text-base/4.5` — the **unprefixed** step, live only below 640px.
///
/// `4.5` is a spacing-scale line height, `calc(var(--spacing) * 4.5)` = 18px,
/// which against a 16px `text-base` is the ratio below.
pub const BASE_STEP: TypeStep = TypeStep {
    size: Rems(1.0),
    line_height: 18.0 / 16.0,
};

/// `sm:text-sm/4` — the step every real Crowbar window renders. 14px text on a
/// `calc(var(--spacing) * 4)` = 16px line box.
pub const SM_STEP: TypeStep = TypeStep {
    size: Rems(0.875),
    line_height: 16.0 / 14.0,
};

/// The `className` bundle a call site merges over the primitive's own.
///
/// Both live importers are settings UI, and they differ in exactly one visual
/// property — the weight — which is why this is a vocabulary rather than a
/// boolean.
#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub enum CallSite {
    /// No className: the primitive's own `font-medium` stands.
    None,
    /// `settings-section.tsx`'s section header,
    /// `ui-font ui-text-base font-medium text-foreground`. The weight agrees
    /// with the primitive's, and `ui-text-base` is dead — see the module docs.
    /// **This is the captured cell.**
    SectionHeader,
    /// `settings-section.tsx`'s row label and
    /// `tabs/sortable-provider-row.tsx`'s provider name, both
    /// `ui-font ui-text-sm … font-normal text-foreground`. The only live
    /// override of a visual property: `font-normal` beats `font-medium`.
    Row,
}

/// Every modelled call site, for `--help` and the closed-vocabulary test.
pub const ALL_CALL_SITES: [CallSite; 3] = [CallSite::None, CallSite::SectionHeader, CallSite::Row];

impl CallSite {
    /// The word `--call-site` takes.
    #[must_use]
    pub fn name(self) -> &'static str {
        match self {
            Self::None => "none",
            Self::SectionHeader => "section-header",
            Self::Row => "row",
        }
    }

    /// The weight this call site pins, if it pins one.
    ///
    /// `font-normal` on the row labels is the **only** visual property any live
    /// call site takes off the primitive. `ui-font` sets a family that resolves
    /// to the same `CalSansUI` the primitive inherits, and `text-foreground`
    /// restates the primitive's own colour.
    #[must_use]
    pub fn weight(self) -> Option<FontWeight> {
        match self {
            Self::None | Self::SectionHeader => None,
            Self::Row => Some(FontWeight::NORMAL),
        }
    }
}

/// One `<Label>`.
#[derive(Clone, Debug, PartialEq)]
pub struct Label {
    /// The string the label paints.
    pub text: SharedString,
    /// The call site whose className is merged over the primitive's.
    pub call_site: CallSite,
    /// Which side of `sm:` the viewport is on.
    pub breakpoint: Breakpoint,
}

impl Label {
    /// The captured cell: the **Typography** section header in the settings
    /// dialog at a 1714px viewport, `80.89 × 16`, `#f5f5f5ff`, `CalSansUI`
    /// 14/16 at weight 500.
    #[must_use]
    pub fn fixture() -> Self {
        Self {
            text: SharedString::new_static("Typography"),
            call_site: CallSite::SectionHeader,
            breakpoint: Breakpoint::Sm,
        }
    }

    /// The type step this cell renders — the whole point of [`Breakpoint`].
    #[must_use]
    pub fn type_step(&self) -> TypeStep {
        match self.breakpoint {
            Breakpoint::Base => BASE_STEP,
            Breakpoint::Sm => SM_STEP,
        }
    }

    /// The weight, after the call site has had its say.
    #[must_use]
    pub fn weight(&self) -> FontWeight {
        self.call_site.weight().unwrap_or(WEIGHT)
    }

    /// The label's own box.
    ///
    /// `inline-flex` becomes `.flex()` for `badge`'s reason: gpui has no inline
    /// flow, and the reference's own computed `display` is **`flex`** anyway —
    /// measured live, because CSS blockifies a flex item's display and the
    /// settings rows make every label one.
    ///
    /// The family is named rather than inherited, for `git_status_row`'s reason:
    /// `ANCHORS.md` v1.2 makes `font.family` the *declared* first family, and an
    /// inherited style reports macOS's `.SystemUIFont`, a string the DOM never
    /// produces. The reference says `CalSansUI`.
    fn shell(&self, theme: &Theme) -> Div {
        let step = self.type_step();
        div()
            .font(ui_sans_font(theme))
            .flex()
            .items_center()
            .gap(GAP)
            .text_size(step.size)
            .line_height(relative(step.line_height))
            .font_weight(self.weight())
            .text_color(theme.color_foreground)
    }

    /// The element, with its one anchor — a painted box *and* a text run under
    /// one id, which `ANCHORS.md` §3 has always permitted.
    pub fn render(&self, theme: &Theme, anchors: &dyn AnchorSink) -> AnyElement {
        let id = AnchorId::from(ID_LABEL).content_sized();
        if self.text.is_empty() {
            // §8.3's `empty`: no text node, so no run and no `font` — and with
            // no `font` the anchor must not declare `line_sized`, which
            // `ANCHORS.md` v1.6 makes a refusal rather than a delta.
            return anchors.boxed(id, self.shell(theme));
        }
        anchors.boxed_text(id.line_sized(), self.shell(theme), self.text.clone())
    }
}

#[cfg(test)]
mod tests {
    use super::{ALL_CALL_SITES, BASE_STEP, CallSite, GAP, ID_LABEL, Label, SM_STEP, WEIGHT};
    use crate::surfaces::rows::git_status_row::{BREAKPOINT_SM, Breakpoint};
    use gpui::{FontWeight, Rems, px};

    /// Both declarations are made, and on the same single anchor. See the module
    /// docs for why `line_sized` is true here where it is false on `badge`.
    #[test]
    fn the_label_is_both_content_sized_and_line_sized() {
        assert_eq!(super::CONTENT_SIZED, [ID_LABEL]);
        assert_eq!(super::LINE_SIZED, [ID_LABEL]);
    }

    /// **The `sm:` step is the live one**, and it is 14px on a 16px line box —
    /// which is what the reference reports and what the call site's
    /// `ui-text-sm` (12px) fails to override. See the module docs.
    #[test]
    fn the_sm_step_wins_at_every_real_window_width() {
        assert_eq!(SM_STEP.size, Rems(0.875));
        assert!((SM_STEP.size.0 * 16.0 - 14.0).abs() < f32::EPSILON);
        // 14 × (16/14) = 16, the reference's `font.line_height`.
        assert!((SM_STEP.size.0 * 16.0 * SM_STEP.line_height - 16.0).abs() < 1e-4);

        // And the dead one really is a different number, so the two cannot be
        // confused by a test that would pass either way.
        assert_eq!(BASE_STEP.size, Rems(1.0));
        assert!((BASE_STEP.size.0 * 16.0 * BASE_STEP.line_height - 18.0).abs() < 1e-4);

        let wide = Label {
            breakpoint: Breakpoint::of(1714.0),
            ..Label::fixture()
        };
        assert_eq!(wide.type_step().size, SM_STEP.size);
        let narrow = Label {
            breakpoint: Breakpoint::of(BREAKPOINT_SM - 1.0),
            ..Label::fixture()
        };
        assert_eq!(narrow.type_step().size, BASE_STEP.size);
    }

    /// `font-normal` on the row labels is the only visual property a live call
    /// site takes off the primitive.
    #[test]
    fn only_the_row_call_site_moves_the_weight() {
        assert_eq!(WEIGHT, FontWeight::MEDIUM);
        for call_site in ALL_CALL_SITES {
            let label = Label {
                call_site,
                ..Label::fixture()
            };
            let expected = match call_site {
                CallSite::Row => FontWeight::NORMAL,
                CallSite::None | CallSite::SectionHeader => FontWeight::MEDIUM,
            };
            assert_eq!(label.weight(), expected, "{}", call_site.name());
        }
    }

    /// `gap-2` is 8px, measured live rather than assumed from the scale.
    #[test]
    fn the_gap_is_the_measured_eight_pixels() {
        assert_eq!(GAP, px(8.0));
    }

    /// The fixture is the captured cell, digit for digit on the fields a
    /// component can hold.
    #[test]
    fn the_fixture_is_the_captured_typography_header() {
        let fixture = Label::fixture();
        assert_eq!(fixture.text, "Typography");
        assert_eq!(fixture.call_site, CallSite::SectionHeader);
        assert_eq!(fixture.breakpoint, Breakpoint::Sm);
        assert_eq!(fixture.weight(), FontWeight::MEDIUM);
    }

    /// The vocabulary is closed and its words are unique.
    #[test]
    fn the_call_site_vocabulary_is_closed() {
        let mut names: Vec<_> = ALL_CALL_SITES.iter().map(|c| c.name()).collect();
        names.sort_unstable();
        assert_eq!(names, ["none", "row", "section-header"]);
    }
}
