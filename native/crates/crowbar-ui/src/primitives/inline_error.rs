//! `inline-error` — a retry panel, and the P3.10 surface that is **structurally
//! unreachable** rather than merely unreached.
//!
//! The native half of `web/src/components/ui/inline-error.tsx`. See
//! `native/mapping/inline-error.md`.
//!
//! # There is no reference, and the reason is stronger than "I could not drive it"
//!
//! `separator` and `skeleton` were reported as having no live instance. This one
//! is a step further: its render guard **cannot become true**, and that was
//! measured rather than argued.
//!
//! Its single call site is `components/layout/workspace-tree.tsx`:
//!
//! `text
//! if (wsListData.status === 'error' && repos.length === 0) return <InlineError … />
//! `
//!
//! `wsListData` is `useWorkspaceListStore`, a `createLoadableSlice`. Reading the
//! slice, there is exactly **one** writer of the error state —
//! `lib/store/loadable-slice.ts:64`, `catch { set({ data: failed(err) }) }` —
//! and it fires only when `cfg.fetcher` rejects. This store's fetcher is
//! `buildTreeFromCache`, whose entire I/O is two `getAllEntities` calls, and
//! `lib/persistence/entity-cache.ts:30` is:
//!
//! `text
//! try { … } catch { return [] }
//! `
//!
//! Everything else in the fetcher is a pure filter and grouping over arrays.
//! **The fetcher cannot reject, so the status cannot become `error`, so the
//! panel cannot mount.** Note in particular that it reads *`IndexedDB` and not the
//! network*: even a dead daemon does not produce this state.
//!
//! Confirmed in the running app rather than only read: `getAllEntities` was
//! called against a bogus store name (which makes `idb`'s `getAll` reject) and
//! returned `[]`; `indexedDB.open` was then replaced with a thrower and a read
//! of a real store still returned its rows, because `getDB()` caches the handle.
//! Both went through the catch arm and neither propagated.
//!
//! So the values below come from the **utilities**, resolved through the app's
//! own compiled CSS and read off a probe element in the live document — the
//! method `native/MAPPING.md` fixes — and **no reference JSON was fabricated**.
//! `git-row-dir` is the precedent: rendered by the port, absent from the product.
//!
//! # `opacity-50` is painted and invisible to the contract
//!
//! The `⚠` glyph carries `opacity-50`. `ANCHORS.md` has no opacity field, and
//! v1.7's `visible` term makes an anchor invisible only at **zero** — so the
//! glyph reports `visible: true` on both sides and the half-transparency is
//! simply not a thing the differ can see. It is applied anyway, because the
//! oracle is a geometry-and-colour oracle and not the definition of correct:
//! `button`'s `disabled:opacity-64` is recorded the same way.
//!
//! # The `<Button>` is composed from `button`'s values, not from a [`Button`]
//!
//! `Button::render` opts the button in through [`AnchorSink::root`], and on the
//! driver-backed sink that is `crowbar_driver::anchor_root`, which **clears the
//! registry** as it enters `prepaint` — that is what makes a snapshot one frame.
//! A `Button::render` nested inside another surface therefore discards every
//! anchor laid out before it, and says nothing: the snapshot is well-formed and
//! its root is simply gone.
//!
//! P3.1 could not meet this — all nine live Buttons are the whole surface — and
//! it is the first thing a *container* surface hits. Rather than grow the
//! primitive an API with one caller, the retry control is built here from
//! `button`'s **public** values: [`Size::Sm`]'s extent, padding, gap, radius and
//! type step, and [`Variant::Outline`]'s three colours. Nothing is re-derived,
//! so nothing can drift; only the flex box is re-spelled.
//! `the_retry_control_reuses_the_button_primitives_values` asserts that.
//!
//! # `line_sized` on three of five, and one of them is v1.6's exact case
//!
//! The glyph, the title and the detail are bare runs in a flex **column** whose
//! `items-center` stops the cross axis stretching them — so each is both
//! `content_sized` (its width is its run's) and `line_sized` (its height *is*
//! its line box). The retry control is neither: `h-8 sm:h-7` authors its height,
//! which is `badge`'s rule.
//!
//! The detail line is worth naming: `text-[11px]` resolves to a `16.5px` line
//! box, and the probe measures the box at **16** — `WebKit` floored it, exactly the
//! asymmetry v1.6 exists for. Declaring `line_sized` is what makes that a 0.5px
//! quantisation instead of a delta.

use gpui::{
    AnyElement, Div, FontWeight, IntoElement as _, ParentElement as _, Pixels, Rems, SharedString,
    Styled as _, div, px, relative,
};

use crate::anchor::{AnchorId, AnchorSink};
use super::badge::TypeStep;
use super::button::{ButtonState, Size, Variant};
use crate::surfaces::rows::git_status_row::Breakpoint;
use crate::theme::{Theme, ui_sans_font};

/// The panel's own box — the surface root.
pub const ID_PANEL: &str = "inline-error";

/// The `⚠` span.
pub const ID_GLYPH: &str = "inline-error-glyph";

/// The `<p>` carrying the title.
pub const ID_TITLE: &str = "inline-error-title";

/// The `<p>` carrying `error.message`.
pub const ID_DETAIL: &str = "inline-error-detail";

/// The retry `<Button>`.
///
/// Named for **this** surface rather than left as `button`'s own id, which is
/// `ANCHORS.md` v1.8 applied the way `git-row-badge` applies it: an anchor
/// beneath the root that belongs to another surface is not part of this one, and
/// a call site that renames it makes it genuinely this component's.
pub const ID_RETRY: &str = "inline-error-retry";

/// Every box on this surface except the panel: `items-center` on a flex column
/// leaves the cross axis unstretched, so each child's width is its own content's.
pub const CONTENT_SIZED: [&str; 4] = [ID_GLYPH, ID_TITLE, ID_DETAIL, ID_RETRY];

/// The three bare runs. **Not the retry control** — `h-8 sm:h-7` authors its
/// height, which `ANCHORS.md` v1.6 makes the test rather than "does it paint
/// text".
pub const LINE_SIZED: [&str; 3] = [ID_GLYPH, ID_TITLE, ID_DETAIL];

/// `p-6` — `calc(var(--spacing) * 6)`, measured `24px` on all four sides.
pub const PADDING: Pixels = px(24.0);

/// `gap-2` — measured `8px` between all four children.
pub const GAP: Pixels = px(8.0);

/// `mt-1` on the retry control — the call site's only className, measured `4px`.
pub const RETRY_MARGIN_TOP: Pixels = px(4.0);

/// `opacity-50` on the glyph. **Painted, and invisible to the contract** — see
/// the module docs.
pub const GLYPH_OPACITY: f32 = 0.5;

/// `text-lg` on the glyph — `18px` on Tailwind's stock `--text-lg--line-height`
/// of `calc(1.75 / 1.125)`, which resolves to the probe's `28px`.
pub const GLYPH_STEP: TypeStep = TypeStep {
    size: Rems(1.125),
    line_height: 28.0 / 18.0,
};

/// `text-sm` on the title — `14px` on `calc(1.25 / 0.875)`, the probe's `20px`.
pub const TITLE_STEP: TypeStep = TypeStep {
    size: Rems(0.875),
    line_height: 20.0 / 14.0,
};

/// `text-[11px]` on the detail — an **arbitrary** value with no paired
/// line-height token, which lands on the inherited `1.5` ratio: the probe reads
/// `font-size: 11px`, `line-height: 16.5px`, and a **box of 16**.
///
/// That gap is v1.6's asymmetry and the reason [`ID_DETAIL`] is `line_sized`.
pub const DETAIL_STEP: TypeStep = TypeStep {
    size: Rems(0.6875),
    line_height: 1.5,
};

/// `font-medium` on the title. The glyph and the detail inherit `400`.
pub const TITLE_WEIGHT: FontWeight = FontWeight::MEDIUM;

/// The `aria-hidden` glyph the panel paints above its title.
pub const GLYPH: &str = "⚠";

/// `title = 'Failed to load'` — the prop's default, and the only string any call
/// site produces: `workspace-tree.tsx` passes no `title`.
pub const DEFAULT_TITLE: &str = "Failed to load";

/// One `<InlineError>`.
#[derive(Clone, Debug, PartialEq)]
pub struct InlineError {
    /// `title`, defaulted at the prop.
    pub title: SharedString,
    /// `error.message`, or `None` for a production build.
    ///
    /// The detail line is behind `import.meta.env.DEV`, so its anchor is present
    /// in a dev build and absent in a shipped one. That makes the anchor **set**
    /// a property of the build rather than of the surface, which is why this
    /// surface declares no set in `oracleSurfaceScope` — `ANCHORS.md` v1.8 allows
    /// a declaration only where the set is the surface's, and a declared anchor
    /// that is legitimately absent is a refusal rather than a delta.
    pub detail: Option<SharedString>,
    /// Which side of the `sm` breakpoint the viewport is on — the retry
    /// control's height moves at it (`h-8 sm:h-7`).
    pub breakpoint: Breakpoint,
}

impl InlineError {
    /// The panel as `workspace-tree.tsx` would render it, in a dev build: the
    /// default title, a short message, at the `sm` breakpoint.
    ///
    /// **The message is one unbreakable token, deliberately.** It is the only
    /// string here a caller does not control, and a run that wraps is outside
    /// what the contract can compare — the DOM sums client rects where gpui
    /// shapes one line. In the 294px sidebar the content box is 246px, so a real
    /// `error.message` would wrap routinely; the fixture is chosen so the cell
    /// stays comparable by construction rather than by luck.
    #[must_use]
    pub fn fixture() -> Self {
        Self {
            title: SharedString::new_static(DEFAULT_TITLE),
            detail: Some(SharedString::new_static("offline")),
            breakpoint: Breakpoint::Sm,
        }
    }

    /// The panel's own box: `flex flex-1 flex-col items-center justify-center
    /// gap-2 p-6 text-center`.
    fn shell(theme: &Theme) -> Div {
        div()
            .font(ui_sans_font(theme))
            .flex()
            .flex_1()
            .flex_col()
            .items_center()
            .justify_center()
            .gap(GAP)
            .p(PADDING)
            .text_color(theme.color_foreground)
    }

    /// The retry control's box, built from [`Size::Sm`] and [`Variant::Outline`].
    fn retry_box(&self, theme: &Theme) -> Div {
        let state = ButtonState::resting();
        let step = Size::Sm.type_step(theme, self.breakpoint);
        let mut element = div()
            .flex()
            .flex_shrink_0()
            .items_center()
            .justify_center()
            .whitespace_nowrap()
            .mt(RETRY_MARGIN_TOP)
            .h(Size::Sm.extent(self.breakpoint))
            .px(Size::Sm.padding_x())
            .gap(Size::Sm.gap())
            .rounded(Size::Sm.radius(theme))
            .border_1()
            .border_color(Variant::Outline.border(theme, state))
            .text_size(step.size)
            .line_height(relative(step.line_height))
            .font_weight(FontWeight::MEDIUM)
            .text_color(Variant::Outline.foreground(theme));

        if let Some(background) = Variant::Outline.background(theme, state) {
            element = element.bg(background);
        }
        element
    }

    /// The label the retry control paints — `↺ Retry`, verbatim from the call
    /// site, glyph included: it is a text node in the JSX and not an `<svg>`, so
    /// unlike every other icon in this port it really is part of the run.
    #[must_use]
    pub fn retry_label() -> SharedString {
        SharedString::new_static("↺ Retry")
    }

    /// The element and its anchors — four in a production build, five in a dev
    /// one.
    pub fn render(&self, theme: &Theme, anchors: &dyn AnchorSink) -> AnyElement {
        let mut element = Self::shell(theme).child(
            anchors.boxed_text(
                AnchorId::from(ID_GLYPH).content_sized().line_sized(),
                div()
                    .opacity(GLYPH_OPACITY)
                    .text_size(GLYPH_STEP.size)
                    .line_height(relative(GLYPH_STEP.line_height)),
                SharedString::new_static(GLYPH),
            ),
        );

        element = element.child(
            anchors.boxed_text(
                AnchorId::from(ID_TITLE).content_sized().line_sized(),
                div()
                    .text_size(TITLE_STEP.size)
                    .line_height(relative(TITLE_STEP.line_height))
                    .font_weight(TITLE_WEIGHT)
                    .text_color(theme.color_foreground),
                self.title.clone(),
            ),
        );

        if let Some(detail) = self.detail.clone() {
            element = element.child(
                anchors.boxed_text(
                    AnchorId::from(ID_DETAIL).content_sized().line_sized(),
                    div()
                        .font_family(theme.font_mono.primary().unwrap_or("monospace"))
                        .text_size(DETAIL_STEP.size)
                        .line_height(relative(DETAIL_STEP.line_height))
                        .text_color(theme.muted_foreground),
                    detail,
                ),
            );
        }

        element = element.child(anchors.boxed_text(
            AnchorId::from(ID_RETRY).content_sized(),
            self.retry_box(theme),
            Self::retry_label(),
        ));

        anchors.root(ID_PANEL.into(), element).into_any_element()
    }
}

#[cfg(test)]
mod tests {
    use super::{
        CONTENT_SIZED, DEFAULT_TITLE, DETAIL_STEP, GAP, GLYPH, GLYPH_STEP, ID_DETAIL, ID_GLYPH,
        ID_PANEL, ID_RETRY, ID_TITLE, InlineError, LINE_SIZED, PADDING, RETRY_MARGIN_TOP,
        TITLE_STEP,
    };
    use crate::primitives::button::{ButtonState, Size, Variant};
    use crate::surfaces::rows::git_status_row::Breakpoint;
    use crate::theme::Theme;
    use gpui::px;

    /// The two declaration lists, and the one anchor deliberately missing from
    /// the second.
    #[test]
    fn the_retry_control_is_content_sized_but_not_line_sized() {
        assert_eq!(CONTENT_SIZED, [ID_GLYPH, ID_TITLE, ID_DETAIL, ID_RETRY]);
        assert_eq!(LINE_SIZED, [ID_GLYPH, ID_TITLE, ID_DETAIL]);
        assert!(CONTENT_SIZED.contains(&ID_RETRY));
        assert!(!LINE_SIZED.contains(&ID_RETRY));
        // The root is in neither: `flex-1` in a column stretches it.
        assert!(!CONTENT_SIZED.contains(&ID_PANEL));
        assert!(!LINE_SIZED.contains(&ID_PANEL));
    }

    /// The retry control's height is **authored** and its line box is smaller, so
    /// `line_sized` would manufacture a delta — `badge`'s rule, asserted as a
    /// distance rather than restated as an opinion.
    #[test]
    fn the_retry_height_is_authored_and_differs_from_its_line_box() {
        let theme = Theme::DARK;
        for breakpoint in [Breakpoint::Base, Breakpoint::Sm] {
            let step = Size::Sm.type_step(&theme, breakpoint);
            let line_box = step.size.0 * 16.0 * step.line_height;
            let extent = f32::from(Size::Sm.extent(breakpoint));
            assert!(
                (extent - line_box).abs() > 0.5,
                "{breakpoint:?}: box {extent} against line box {line_box}",
            );
        }
    }

    /// The three runs' boxes really are their line boxes, which is what makes the
    /// `line_sized` declaration true rather than hopeful.
    ///
    /// The detail step is the interesting one: `11 × 1.5` is **16.5**, which
    /// `WebKit` floors into a 16px box — the asymmetry v1.6 exists for.
    #[test]
    fn every_line_sized_run_resolves_to_the_probed_line_box() {
        let glyph = GLYPH_STEP.size.0 * 16.0 * GLYPH_STEP.line_height;
        assert!((glyph - 28.0).abs() < 1e-3, "{glyph}");

        let title = TITLE_STEP.size.0 * 16.0 * TITLE_STEP.line_height;
        assert!((title - 20.0).abs() < 1e-3, "{title}");

        let detail = DETAIL_STEP.size.0 * 16.0 * DETAIL_STEP.line_height;
        assert!((detail - 16.5).abs() < 1e-3, "{detail}");
        // The probe measured the box at 16: `WebKit` floors, gpui snaps.
        assert!(detail.floor() < detail, "the floor is what v1.6 forgives");
    }

    /// Every value on the retry control comes from `button`'s own vocabulary, so
    /// a change there moves this component too.
    ///
    /// The point of the test is that these are **reads, not copies**: it would
    /// fail if someone replaced a call with the literal it happens to return
    /// today.
    #[test]
    fn the_retry_control_reuses_the_button_primitives_values() {
        let theme = Theme::DARK;
        let state = ButtonState::resting();
        assert_eq!(Size::Sm.extent(Breakpoint::Sm), px(28.0));
        assert_eq!(Size::Sm.extent(Breakpoint::Base), px(32.0));
        assert_eq!(Size::Sm.radius(&theme), theme.radius_lg.value());
        assert_eq!(
            Variant::Outline.border(&theme, state),
            theme.input,
            "`outline` borders with --input, and it is a painted 1px",
        );
        assert_eq!(Variant::Outline.foreground(&theme), theme.color_foreground);
    }

    /// The spacing the probe measured.
    #[test]
    fn the_panel_spacing_is_the_probed_values() {
        assert_eq!(PADDING, px(24.0));
        assert_eq!(GAP, px(8.0));
        assert_eq!(RETRY_MARGIN_TOP, px(4.0));
    }

    /// The dev-only detail is the anchor **set** moving with the build, which is
    /// why this surface declares no set on the reference side.
    #[test]
    fn a_production_build_drops_the_detail_anchor() {
        let dev = InlineError::fixture();
        assert!(dev.detail.is_some());
        let shipped = InlineError {
            detail: None,
            ..InlineError::fixture()
        };
        assert!(shipped.detail.is_none());
        assert_ne!(dev, shipped);
    }

    /// The fixture is the only picture the call site can produce, and neither of
    /// its strings can wrap.
    #[test]
    fn the_fixture_is_the_only_shape_the_call_site_produces() {
        let fixture = InlineError::fixture();
        assert_eq!(fixture.title, DEFAULT_TITLE);
        assert_eq!(GLYPH, "⚠");
        assert!(
            !fixture.detail.as_ref().expect("dev build").contains(' '),
            "a message with a break opportunity wraps in a 246px content box",
        );
    }
}
