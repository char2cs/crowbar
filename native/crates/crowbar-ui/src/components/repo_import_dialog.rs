//! `repo-import-dialog` — a **call site** of `dialog.tsx`'s own primitive
//! with a viewport-relative height, and no footer at all.
//!
//! `web/src/components/layout/repo-import-dialog.tsx` renders through the
//! same `Dialog`/`DialogPopup`/`DialogHeader`/`DialogTitle`/
//! `DialogDescription` primitive `dialog.rs` already wraps — see
//! `detach_holder_modal`'s module docs for why the *real* DOM this call site
//! paints therefore carries `dialog-*` ids, not a namespace of its own, and
//! why this module's own ids differ anyway (a registry constraint —
//! `native/mapping/repo-import-dialog.md` §0). It never nests a
//! `DialogFooter`, so there is no footer concept in this module at all.
//!
//! # The one real shape this call site adds: a viewport-relative popup height
//!
//! `DialogPopup className="flex h-[70vh] max-w-md flex-col p-0"`.
//! `h-[70vh]` sets the popup's own height to 70% of the *window*, not to the
//! header/body/footer sum `dialog::Dialog::popup_height` derives it as —
//! `dialog.md` §5 names this exact call site as the reason: *"a viewport-
//! relative height, unlike either reachable reference's content-driven one …
//! read `min_h_24()`-style arithmetic off it before trusting `Dialog::
//! fixture()` for that specific call site."* This module inverts `dialog`'s
//! own relationship: [`RepoImportDialog::popup_height`] is the **input**
//! (this component's own free variable), and the space below the header is
//! the **derived** remainder — see [`RepoImportDialog::body_height`].
//!
//! `flex-col` (already `dialog`'s primitive default) and `p-0` (a no-op: the
//! primitive's own popup carries no padding of its own to zero — every real
//! padding is the header's/footer's own, exactly as `dialog.md` §"the popup
//! box is ours" records) contribute nothing new.
//!
//! # Why this port does not read a live window's height, unlike its width
//!
//! `dialog::Dialog::render` reproduces `w-full max-w-*` by reading
//! `window.viewport_size().width`, because `crowbar-app`'s row-layout harness
//! carries an **independent** `--viewport-width` axis (`row_surface.rs`'s own
//! `Cell::viewport_width`) — a window can be asked to be exactly as wide as a
//! `sm:` breakpoint decision needs. There is **no vertical equivalent**:
//! `Surface::min_window_height`/`SurfaceParams::driven_height` make the
//! window exactly as tall as the surface's *own* declared content needs, so
//! asking `window.viewport_size().height` from inside `render` would close a
//! loop this harness has no independent input to break — the window's height
//! is an **output** of what this struct declares, never an input to it.
//! `native/mapping/repo-import-dialog.md` §2 records this asymmetry in full.
//!
//! So `h-70vh`'s live responsiveness is not reproduced;
//! [`RepoImportDialog::popup_height`] is a plain field, exactly
//! `dialog::Dialog::body_height` is one — "the port takes its measured
//! extent," restated one level up because here it is the *whole popup*, not
//! just its body, that this call site's own CSS makes exogenous to content.
//!
//! # The body is unmodelled, and more of it than `dialog`'s ever was
//!
//! Between the header and the (absent) footer, the real call site renders a
//! search `Input`, a virtualized, network-fetched branch list, a hint
//! paragraph and an `Import` button — substantial, but every bit of it is
//! this call site's *own* content in exactly the sense `dialog.md` §3 and
//! `primitive-dialog-service.md` §4 both already establish for less content:
//! "the port takes its measured extent rather than reproducing one of them."
//! Reproducing the list in particular would mean modelling network-fetched,
//! unbounded-count rows this port cannot pin to any reference regardless.
//! `native/mapping/repo-import-dialog.md` §4 names precisely what a future
//! item would need (its own `data-oracle-id`s on the search input, the list
//! viewport, the hint paragraph and the button — none of which
//! `repo-import-dialog.tsx` carries today) rather than silently folding the
//! gap into "body, measured."

use gpui::{
    AnyElement, App, Div, FontWeight, IntoElement as _, ParentElement as _, Pixels, SharedString,
    Styled as _, Window, div, px, relative,
};
use gpui_component::dialog::Dialog as GpuiDialog;

use super::anchor::{AnchorId, AnchorSink};
use crate::theme::{Color, Theme};

/// The root anchor. **Not** `"dialog-popup"` — see the module docs' pointer
/// to `detach_holder_modal`'s identical finding.
pub const ID_POPUP: &str = "repo-import-dialog-popup";
/// `DialogHeader`, always rendered — the one live call site always nests a
/// title and a description.
pub const ID_HEADER: &str = "repo-import-dialog-header";
/// `DialogTitle`.
pub const ID_TITLE: &str = "repo-import-dialog-title";
/// `DialogDescription`.
pub const ID_DESCRIPTION: &str = "repo-import-dialog-description";

/// See `dialog::CONTENT_SIZED` — identical reasoning, identical empty answer.
pub const CONTENT_SIZED: [&str; 0] = [];

/// See `dialog::LINE_SIZED` — the title, and only the title.
pub const LINE_SIZED: [&str; 1] = [ID_TITLE];

/// `border` on the popup. **Identical to `dialog::BORDER_WIDTH`.**
pub const BORDER_WIDTH: Pixels = px(1.0);

/// `p-4`'s top/left/right — **not** `dialog::HEADER_PADDING`'s 24px. This
/// call site's own header override, `p-4 pb-2`, not `add-repository-modal`'s
/// unmodified `p-6`.
pub const HEADER_PADDING: Pixels = px(16.0);

/// `pb-2` — the bottom side `p-4` does not win on tailwind-merge, an
/// axis-specific override the identical shape `detach-holder-modal`'s
/// `pr-10` is, on the opposite side of the box.
pub const HEADER_PADDING_BOTTOM: Pixels = px(8.0);

/// `gap-2` inside the header. **Identical to `dialog::HEADER_GAP`**, and — as
/// on `detach-holder-modal` — live here: this call site always renders both
/// a title and a description.
pub const HEADER_GAP: Pixels = px(8.0);

/// `DialogViewport`'s `p-4`. **Identical to `dialog::VIEWPORT_PADDING`.**
pub const VIEWPORT_PADDING: Pixels = px(16.0);

/// `leading-none` on the title. **Identical to `dialog`'s.**
const TITLE_LINE_HEIGHT: f32 = 1.0;

/// Inherited `text-base`'s line height on the popup. **Identical to
/// `dialog::POPUP_LINE_HEIGHT`.**
const POPUP_LINE_HEIGHT: f32 = 1.5;

/// `text-sm`'s stock line height on the description — this call site takes
/// no `leading-*` override, unlike `detach-holder-modal`'s. **Identical to
/// `dialog::DESCRIPTION_LINE_HEIGHT`.**
const DESCRIPTION_LINE_HEIGHT: f32 = 1.25 / 0.875;

/// `h-[70vh]`'s own factor: 70% of the assumed window height. Spelled as a
/// constant rather than folded into a literal so [`popup_height_at`] reads
/// as the source's own arithmetic rather than an opaque number.
const VIEWPORT_HEIGHT_FACTOR: f32 = 0.7;

/// `DialogPopup`, `DialogHeader`, `DialogTitle` and `DialogDescription`, as
/// this one call site configures them. No footer concept — this call site
/// never nests a `DialogFooter`.
#[derive(Clone, Debug, PartialEq)]
pub struct RepoImportDialog {
    /// `max-w-md` (no `sm:` prefix) — the same numeric Tailwind step as
    /// `dialog`'s and `detach-holder-modal`'s own, so at every width this
    /// port drives the three resolve identically. The popup's actual width
    /// is `min(viewport − 2·VIEWPORT_PADDING, max_width)`.
    pub max_width: Pixels,
    /// `h-[70vh]`'s **own** height — this call site's independent variable.
    /// See the module docs for why this port takes it as a plain field
    /// rather than reading a live window.
    pub popup_height: Pixels,
    /// `DialogTitle` — `"Import branches"`, the real, literal, **static**
    /// source string (unlike `detach-holder-modal`'s interpolated one).
    pub title: Option<SharedString>,
    /// `DialogDescription` — `"Bring remote branches into Crowbar as
    /// workspaces."`, the real, literal, static source string.
    pub description: Option<SharedString>,
}

impl RepoImportDialog {
    /// The live `repo-import-dialog.tsx`'s header — both strings are the
    /// real, literal, non-interpolated source text. [`Self::popup_height`]
    /// defaults to [`popup_height_at`] a 900px assumed window height: no live
    /// window-height reference exists for this session (the brief's own hard
    /// constraint against launching the GUI), and `native/mapping/command.md`
    /// records this machine's own logical screen height at 982px — 900 is a
    /// conservative, headroom-adjusted round number under that, **not** a
    /// measurement. `native/mapping/repo-import-dialog.md` §2 says so again
    /// in full; the row-layout tests sweep several window heights rather than
    /// asserting this one default is the "real" one.
    #[must_use]
    pub fn fixture() -> Self {
        Self {
            max_width: px(448.0),
            popup_height: popup_height_at(px(900.0)),
            title: Some(SharedString::new_static("Import branches")),
            description: Some(SharedString::new_static(
                "Bring remote branches into Crowbar as workspaces.",
            )),
        }
    }

    /// The header's own height: padding plus its content.
    ///
    /// Used only to size the row-layout harness's window tall enough to
    /// clear the header — the description here is short (one sentence) and
    /// the row-layout test still reads its **real** rendered height back
    /// rather than trusting this arithmetic, for the identical caution
    /// `detach_holder_modal::popup_height_estimate`'s doc names for its own
    /// (much longer) description.
    #[must_use]
    pub fn header_height_estimate(&self, theme: &Theme) -> Pixels {
        if self.title.is_none() && self.description.is_none() {
            return px(0.0);
        }
        let mut header = f32::from(HEADER_PADDING) + f32::from(HEADER_PADDING_BOTTOM);
        let mut sections = 0;
        if self.title.is_some() {
            header += theme.ui_text_xl.value().0 * 16.0 * TITLE_LINE_HEIGHT;
            sections += 1;
        }
        if self.description.is_some() {
            header += theme.ui_text_base.value().0 * 16.0 * DESCRIPTION_LINE_HEIGHT;
            sections += 1;
        }
        if sections > 1 {
            header += f32::from(HEADER_GAP);
        }
        px(header)
    }

    /// The body's own height: the popup's content height less the header's.
    /// **Derived**, the inverse of `dialog::Dialog::body_height`'s role
    /// (there an input; here an output) — see the module docs.
    #[must_use]
    pub fn body_height(&self, theme: &Theme) -> Pixels {
        let content_height = f32::from(self.popup_height) - f32::from(BORDER_WIDTH) * 2.0;
        px((content_height - f32::from(self.header_height_estimate(theme))).max(0.0))
    }

    /// Renders the popup through `gpui_component::Dialog` — the same vendor
    /// type `dialog::Dialog::render` wraps, with the identical
    /// neutralisation.
    #[must_use]
    pub fn render(
        &self,
        window: &mut Window,
        cx: &mut App,
        theme: &Theme,
        anchors: &dyn AnchorSink,
    ) -> AnyElement {
        let family = theme.font_sans.primary().unwrap_or("sans-serif");

        let mut inner = div()
            .flex()
            .flex_col()
            .size_full()
            .rounded(theme.radius_2xl.value())
            .border(BORDER_WIDTH)
            .border_color(theme.border)
            .bg(theme.popover)
            .text_color(theme.popover_foreground)
            .font_family(family)
            .font_weight(FontWeight::NORMAL)
            .text_size(theme.ui_text_lg.value())
            .line_height(relative(POPUP_LINE_HEIGHT));

        if self.title.is_some() || self.description.is_some() {
            inner = inner.child(self.header(theme, anchors));
        }
        inner = inner.child(self.body(theme));

        let popup = anchors.root(ID_POPUP.into(), inner);

        // `w-full max-w-*` by hand — see `dialog::Dialog::render`'s identical
        // comment. This is the one axis this call site still drives against
        // a live window: the module docs explain why the vertical one is not.
        let viewport_width = f32::from(window.viewport_size().width);
        let outer_width = px((viewport_width - f32::from(VIEWPORT_PADDING) * 2.0)
            .min(f32::from(self.max_width))
            .max(0.0));

        GpuiDialog::new(cx)
            .overlay(false)
            .close_button(false)
            .p_0()
            .bg(Color::TRANSPARENT)
            .border_0()
            .border_color(Color::TRANSPARENT)
            .rounded(px(0.0))
            // `min_h_24()` — neutralised for the identical reason `dialog`'s
            // and `alert-dialog`'s own render methods carry this line.
            .min_h(px(0.0))
            .h(self.popup_height)
            .w(outer_width)
            .children([popup])
            .into_any_element()
    }

    /// `DialogHeader`: `flex flex-col gap-2 p-4 pb-2` — top/left/right at
    /// this call site's own 16px, bottom at its own 8px.
    fn header(&self, theme: &Theme, anchors: &dyn AnchorSink) -> AnyElement {
        let mut header = div()
            .flex()
            .flex_col()
            .gap(HEADER_GAP)
            .pt(HEADER_PADDING)
            .pl(HEADER_PADDING)
            .pr(HEADER_PADDING)
            .pb(HEADER_PADDING_BOTTOM);

        if let Some(title) = &self.title {
            header = header.child(anchors.boxed_text(
                AnchorId::new(ID_TITLE).line_sized(),
                Self::title_box(theme),
                title.clone(),
            ));
        }
        if let Some(description) = &self.description {
            header = header.child(anchors.boxed_text(
                AnchorId::new(ID_DESCRIPTION),
                Self::description_box(theme),
                description.clone(),
            ));
        }
        anchors.boxed(ID_HEADER.into(), header)
    }

    /// `DialogTitle`: `font-heading font-semibold text-xl leading-none`.
    /// **Identical to `dialog::Dialog::title_box`** — same dead
    /// `font-heading` utility, same inheritance.
    fn title_box(theme: &Theme) -> Div {
        div()
            .text_size(theme.ui_text_xl.value())
            .line_height(relative(TITLE_LINE_HEIGHT))
            .font_weight(FontWeight::SEMIBOLD)
    }

    /// `DialogDescription`: `text-muted-foreground text-sm` — no
    /// `leading-*` override on this call site, unlike `detach-holder-
    /// modal`'s.
    fn description_box(theme: &Theme) -> Div {
        div()
            .w_full()
            .text_size(theme.ui_text_base.value())
            .line_height(relative(DESCRIPTION_LINE_HEIGHT))
            .text_color(theme.muted_foreground)
    }

    /// The call site's own content — search input, branch list, hint text,
    /// import button — as the box it occupies. **Unanchored on purpose**;
    /// see the module docs for what a future item would need to go further.
    fn body(&self, theme: &Theme) -> Div {
        div().w_full().h(self.body_height(theme))
    }
}

/// `h-[70vh]`, applied to an assumed window height. A free function (not a
/// method) because it is the one piece of this call site's arithmetic that
/// is legitimately a function of an *externally supplied* number, never of
/// `self` — see the module docs' "why this port does not read a live
/// window's height."
#[must_use]
pub fn popup_height_at(window_height: Pixels) -> Pixels {
    px(f32::from(window_height) * VIEWPORT_HEIGHT_FACTOR)
}

#[cfg(test)]
mod tests {
    use super::{
        BORDER_WIDTH, CONTENT_SIZED, HEADER_GAP, HEADER_PADDING, HEADER_PADDING_BOTTOM,
        ID_DESCRIPTION, ID_HEADER, ID_POPUP, ID_TITLE, LINE_SIZED, RepoImportDialog,
        VIEWPORT_PADDING, popup_height_at,
    };
    use crate::theme::Theme;
    use gpui::px;

    /// Every length, against the `calc(var(--spacing) * n)` the app's own
    /// Tailwind compiles the class to.
    #[test]
    fn every_length_is_the_compiled_spacing_multiple() {
        const STEP: f32 = 4.0;

        assert_eq!(HEADER_PADDING, px(STEP * 4.0)); // p-4
        assert_eq!(HEADER_PADDING_BOTTOM, px(STEP * 2.0)); // pb-2
        assert_eq!(HEADER_GAP, px(STEP * 2.0)); // gap-2
        assert_eq!(VIEWPORT_PADDING, px(STEP * 4.0)); // p-4 (DialogViewport)
        assert_eq!(BORDER_WIDTH, px(1.0));
    }

    /// `h-[70vh]`: 70% of whatever window height it is asked about.
    #[test]
    fn popup_height_is_seventy_percent_of_the_window() {
        assert_eq!(popup_height_at(px(900.0)), px(630.0));
        assert_eq!(popup_height_at(px(1000.0)), px(700.0));
        assert_eq!(popup_height_at(px(0.0)), px(0.0));
    }

    /// `HEADER_PADDING`/`HEADER_PADDING_BOTTOM` genuinely differ from
    /// `dialog`'s own 24px on every side — the point of this call site's own
    /// module existing.
    #[test]
    fn the_header_padding_genuinely_differs_from_dialogs_default() {
        assert_ne!(HEADER_PADDING, crate::components::dialog::HEADER_PADDING);
        assert_ne!(
            HEADER_PADDING_BOTTOM,
            crate::components::dialog::HEADER_PADDING
        );
        assert_eq!(HEADER_GAP, crate::components::dialog::HEADER_GAP);
    }

    /// The fixture is the live `repo-import-dialog.tsx` — both header
    /// strings are the real, non-interpolated source text.
    #[test]
    fn the_fixture_is_the_live_repo_import_dialog() {
        let fixture = RepoImportDialog::fixture();

        assert_eq!(fixture.max_width, px(448.0));
        assert_eq!(fixture.popup_height, px(630.0));
        assert_eq!(fixture.title.as_deref(), Some("Import branches"));
        assert_eq!(
            fixture.description.as_deref(),
            Some("Bring remote branches into Crowbar as workspaces."),
        );
    }

    /// **The title is the one line-sized anchor.**
    #[test]
    fn only_the_title_is_line_sized() {
        assert_eq!(LINE_SIZED, [ID_TITLE]);
        assert_eq!(CONTENT_SIZED, [] as [&str; 0]);
    }

    /// The body is the popup's content height less the header's — the
    /// inverse relationship `dialog`'s `popup_height` holds, arithmetic
    /// spot-checked at several popup heights.
    #[test]
    fn the_body_is_the_popup_less_its_borders_and_header() {
        let theme = Theme::DARK;
        for window_height in [500u16, 900, 1400] {
            let fixture = RepoImportDialog {
                popup_height: popup_height_at(px(f32::from(window_height))),
                ..RepoImportDialog::fixture()
            };
            let header = fixture.header_height_estimate(&theme);
            let expected_body =
                f32::from(fixture.popup_height) - f32::from(BORDER_WIDTH) * 2.0 - f32::from(header);
            assert!(
                (f32::from(fixture.body_height(&theme)) - expected_body).abs() < f32::EPSILON,
                "{window_height}",
            );
        }
    }

    /// A popup too short for its own header clamps the body at zero rather
    /// than going negative.
    #[test]
    fn a_popup_shorter_than_its_header_clamps_the_body_at_zero() {
        let theme = Theme::DARK;
        let fixture = RepoImportDialog {
            popup_height: px(10.0),
            ..RepoImportDialog::fixture()
        };
        assert_eq!(fixture.body_height(&theme), px(0.0));
    }

    /// Every anchor id is distinct, namespaced under `repo-import-dialog-`
    /// and never collides with `dialog::ID_*`. No `ID_FOOTER` exists on this
    /// surface at all — this call site never renders one.
    #[test]
    fn every_anchor_id_is_distinct_and_namespaced() {
        let ids = [ID_POPUP, ID_HEADER, ID_TITLE, ID_DESCRIPTION];
        let mut sorted = ids.to_vec();
        sorted.sort_unstable();
        let before = sorted.len();
        sorted.dedup();
        assert_eq!(sorted.len(), before, "{ids:?}");
        assert!(ids.iter().all(|id| id.starts_with("repo-import-dialog-")));
        assert!(
            ids.iter()
                .all(|id| *id != crate::components::dialog::ID_POPUP)
        );
    }
}
