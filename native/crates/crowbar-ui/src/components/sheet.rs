//! `sheet` — the third §6.2 wrap, and the one with **no live call site at
//! all**.
//!
//! `web/src/components/ui/sheet.tsx` → `gpui_component::Sheet` +
//! `DialogHeader`/`DialogTitle`/`DialogDescription` (`sheet.tsx` re-styles
//! the *dialog* primitives' header/title/description verbatim — same class
//! strings, no `Sheet*` equivalents of its own beyond `SheetHeader`, whose
//! class list is byte-identical to `DialogHeader`'s). This module models
//! `SheetPopup` and the header/title/description a call site nests inside
//! it, the same primitive-first choice `dialog` makes.
//!
//! # ‼️ FINDING: the one importer never mounts it
//!
//! `sheet.tsx`'s only consumer anywhere in `web/src` is
//! `components/ui/sidebar.tsx`'s `Sidebar`, behind `if (isMobile)`. `Sidebar`
//! itself — as opposed to `SidebarProvider`/`useSidebar`, which several
//! feature files use for their own collapse state — is never instantiated:
//! `grep -rn '<Sidebar\b' web/src` finds nothing outside `sidebar.tsx` and
//! its own test file. **Live count: zero.** Verified live, not inferred:
//! resizing the running app's window to 700 logical px (well under
//! Tailwind's 768px `md` breakpoint `useMediaQuery('max-md')` reads)
//! produces no sheet, no matter what is clicked, because there is no
//! `Sidebar` on screen to switch into its mobile rendering.
//!
//! This module is written and tested anyway — "port it and say so" — but no
//! `native/oracle/runs/` entry exists for it and none should be fabricated:
//! there is nothing to converge against.
//!
//! # What resisted, precisely — worse than `dialog`'s three points
//!
//! 1. **The border width is not reachable through `refine_style` at all.**
//!    `Dialog`'s `.border_1()` runs *before* `refine_style`, so `.border_0()`
//!    overwrites it. `Sheet`'s per-side border (`.border_l_1()` /
//!    `.border_r_1()` / …) is chosen *inside* a `.map(|this| match
//!    self.placement { … })` that runs **after** `refine_style` — so nothing
//!    this crate sets survives it. It costs nothing today only by
//!    coincidence: Tailwind's bare `border-s`/`border-e` is 1px too, the same
//!    width the vendor's `_1()` suffix always means.
//! 2. **`Sheet.placement` is `pub(crate)`, with no public setter reachable
//!    without a mounted `Root`.** `WindowExt::open_sheet_at(placement, …)` is
//!    the only public way to choose one, and it stores the sheet on
//!    `Root::active_sheet` — the same `Root` dependency `dialog::Dialog`
//!    avoids by never calling `.trigger()`. Constructing `GpuiSheet::new`
//!    directly, as this module does, always gets `Sheet::new`'s own default:
//!    **`Placement::Right`**, unconditionally. This port therefore models
//!    only that one placement; `sheet.tsx`'s `Left`/`Top`/`Bottom` classes are
//!    read and translated in this module's docs but not reachable in
//!    `render`'s output.
//! 3. **The vendor renders a title bar this crate cannot suppress.** Unlike
//!    `Dialog::close_button(bool)`, `Sheet` has no method that removes its
//!    own `h_flex().justify_between()` row — a close [`gpui_component::Button`]
//!    beside whatever `.title()` was given, *unconditionally*, above
//!    whatever this crate's own `.children()` supplies. `sheet.tsx` has no
//!    such row at all: `SheetHeader` is an entirely optional slot a call site
//!    places among its own children, and the close affordance is a
//!    `SheetPrimitive.Close` floated `absolute end-2 top-2`, not a title-bar
//!    button. There is no configuration that reaches parity here — the
//!    vendor's title bar paints regardless, above this module's own
//!    `sheet-header`, not in its place.
//! 4. **The panel's cross axis (height, for `Right`) is not content-driven.**
//!    `Sheet::render` positions it `top(margin_top)…bottom_0()` — an
//!    absolute box between two fixed edges, not `auto`-height — where
//!    `sheet.tsx`'s own `side="right"` sheet has *no* top offset at all
//!    (full window height). [`Sheet::body_height`] is still rendered as this
//!    module's own unanchored box, exactly as `dialog`'s is, but it does not
//!    drive `sheet-popup`'s own height the way `dialog`'s drives
//!    `dialog-popup`'s — the outer's height comes from the vendor's
//!    positioning regardless of content, which is a fact about this
//!    placement rather than a bug in this module.
//!
//! None of the four is visible to the oracle today, because there is nothing
//! on the other side to compare against — but a future item that does find a
//! live sheet, or a way to reach `Root`, should read this list before
//! trusting the port past `Placement::Right`.

use gpui::{
    AnyElement, App, Div, FontWeight, IntoElement as _, ParentElement as _, Pixels, SharedString,
    Styled as _, Window, div, px, relative,
};
use gpui_component::sheet::Sheet as GpuiSheet;

use super::anchor::{AnchorId, AnchorSink};
use crate::theme::{Color, Theme};

/// The root anchor: `SheetPrimitive.Popup`.
pub const ID_POPUP: &str = "sheet-popup";
/// `SheetHeader`, rendered only where a call site nests one.
pub const ID_HEADER: &str = "sheet-header";
/// `SheetTitle`.
pub const ID_TITLE: &str = "sheet-title";
/// `SheetDescription`.
pub const ID_DESCRIPTION: &str = "sheet-description";

/// See `dialog`'s equivalent — the same arithmetic, the same reason.
pub const CONTENT_SIZED: [&str; 0] = [];
/// See `dialog`'s equivalent.
pub const LINE_SIZED: [&str; 1] = [ID_TITLE];

/// `border-s` — bare, so 1px. See the module docs' point 1: this cannot
/// actually be overridden through this crate, and is a coincidence rather
/// than an override that reached anything.
pub const BORDER_WIDTH: Pixels = px(1.0);

/// `SheetHeader`'s `p-6`: identical to `DialogHeader`'s.
pub const HEADER_PADDING: Pixels = px(24.0);

/// `SheetHeader`'s `gap-2`.
pub const HEADER_GAP: Pixels = px(8.0);

/// `--spacing(12)`: the fixed margin `w-[calc(100%-(--spacing(12)))]`
/// reserves on the panel's main axis.
pub const EDGE_MARGIN: Pixels = px(48.0);

/// `max-w-md`, the side sheet's own cap.
pub const MAX_SIZE: Pixels = px(448.0);

const TITLE_LINE_HEIGHT: f32 = 1.0;
const POPUP_LINE_HEIGHT: f32 = 1.5;
const DESCRIPTION_LINE_HEIGHT: f32 = 1.25 / 0.875;

/// `SheetPopup` plus `SheetHeader`/`SheetTitle`/`SheetDescription`, at the
/// only placement `render` can actually produce — see the module docs' point
/// 2.
///
/// **Unreached** — see the module docs. Every field is modelled the way
/// `dialog`'s equivalent is, on the same reasoning, with no live number to
/// measure any of it against.
#[derive(Clone, Debug, PartialEq)]
pub struct Sheet {
    /// The call site's own content, between the header and the panel's
    /// trailing edge. Rendered, but does not drive the popup's own height —
    /// see the module docs' point 4.
    pub body_height: Pixels,
    /// `SheetTitle`, when a call site renders one.
    pub title: Option<SharedString>,
    /// `SheetDescription`, when a call site renders one.
    pub description: Option<SharedString>,
}

impl Sheet {
    /// No live reference: one title, no description, an arbitrary 200px
    /// body.
    #[must_use]
    pub fn fixture() -> Self {
        Self {
            body_height: px(200.0),
            title: Some(SharedString::new_static("Sidebar")),
            description: None,
        }
    }

    /// Renders the panel through `gpui_component::Sheet`.
    ///
    /// `window`/`cx` are needed for the same reason `dialog::Dialog::render`
    /// needs them: `GpuiSheet::new` mints a `FocusHandle` off `cx`.
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
        inner = inner.child(self.body());

        let popup = anchors.root(ID_POPUP.into(), inner);

        // `w-[calc(100%-(--spacing(12)))] max-w-md` by hand, on the window's
        // main axis for `Placement::Right` — the only placement this
        // construction path can produce (module docs, point 2). `+
        // BORDER_WIDTH` compensates the vendor's own un-overridable
        // `border_l_1()` (module docs, point 1) — measured, not assumed:
        // `row_layout/sheet.rs`'s width test was first written without it and
        // read `447` against this crate's own `448` claim, one pixel short on
        // the one side that carries a border. `dialog`'s popup needed no such
        // compensation because its border *is* reachable through
        // `refine_style`; this one is not.
        let viewport_width = f32::from(window.viewport_size().width);
        let content_width = (viewport_width - f32::from(EDGE_MARGIN))
            .min(f32::from(MAX_SIZE))
            .max(0.0);
        let main = px(content_width + f32::from(BORDER_WIDTH));

        GpuiSheet::new(window, cx)
            .overlay(false)
            .resizable(false)
            .p_0()
            .bg(Color::TRANSPARENT)
            .border_0()
            .border_color(Color::TRANSPARENT)
            .size(main)
            .children([popup])
            .into_any_element()
    }

    /// `SheetHeader`: identical shape to `dialog`'s.
    fn header(&self, theme: &Theme, anchors: &dyn AnchorSink) -> AnyElement {
        let mut header = div().flex().flex_col().gap(HEADER_GAP).p(HEADER_PADDING);

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

    /// `SheetTitle`: `font-heading font-semibold text-xl leading-none` — see
    /// `dialog::Dialog::title_box`'s docs for why no family is set here.
    fn title_box(theme: &Theme) -> Div {
        div()
            .text_size(theme.ui_text_xl.value())
            .line_height(relative(TITLE_LINE_HEIGHT))
            .font_weight(FontWeight::SEMIBOLD)
    }

    /// `SheetDescription`: `text-muted-foreground text-sm`.
    fn description_box(theme: &Theme) -> Div {
        div()
            .w_full()
            .text_size(theme.ui_text_base.value())
            .line_height(relative(DESCRIPTION_LINE_HEIGHT))
            .text_color(theme.muted_foreground)
    }

    /// The call site's own content, unanchored — see `dialog::Dialog::body`.
    fn body(&self) -> Div {
        div().w_full().h(self.body_height)
    }
}

#[cfg(test)]
mod tests {
    use super::{
        BORDER_WIDTH, CONTENT_SIZED, EDGE_MARGIN, HEADER_GAP, HEADER_PADDING, ID_DESCRIPTION,
        ID_HEADER, ID_POPUP, ID_TITLE, LINE_SIZED, MAX_SIZE, Sheet,
    };
    use gpui::px;

    #[test]
    fn every_length_is_the_compiled_spacing_multiple() {
        const STEP: f32 = 4.0;

        assert_eq!(HEADER_PADDING, px(STEP * 6.0)); // p-6
        assert_eq!(HEADER_GAP, px(STEP * 2.0)); // gap-2
        assert_eq!(EDGE_MARGIN, px(STEP * 12.0)); // --spacing(12)
        assert_eq!(MAX_SIZE, px(448.0)); // max-w-md
        assert_eq!(BORDER_WIDTH, px(1.0));
    }

    #[test]
    fn the_fixture_is_a_declared_picture_with_no_live_reference() {
        let fixture = Sheet::fixture();
        assert_eq!(fixture.body_height, px(200.0));
        assert_eq!(fixture.title.as_deref(), Some("Sidebar"));
        assert_eq!(fixture.description, None);
    }

    /// The title is the one line-sized anchor, on the same arithmetic
    /// `dialog`'s title is.
    #[test]
    fn only_the_title_is_line_sized() {
        assert_eq!(LINE_SIZED, [ID_TITLE]);
        assert_eq!(CONTENT_SIZED, [] as [&str; 0]);
    }

    #[test]
    fn every_anchor_id_is_distinct_and_namespaced() {
        let ids = [ID_POPUP, ID_HEADER, ID_TITLE, ID_DESCRIPTION];
        let mut sorted = ids.to_vec();
        sorted.sort_unstable();
        let before = sorted.len();
        sorted.dedup();
        assert_eq!(sorted.len(), before, "{ids:?}");
        assert!(ids.iter().all(|id| id.starts_with("sheet-")));
    }
}
