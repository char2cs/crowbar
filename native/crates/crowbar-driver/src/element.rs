//! The anchor elements, and the registry they write into.
//!
//! gpui has no "walk the laid-out tree and read computed styles" API, and no
//! computed style to read — it has an already-*resolved* [`Style`]. So the
//! extractor cannot inspect a frame after the fact; it has to **participate in
//! the element lifecycle**. That is what these two elements are: transparent
//! wrappers that return their child's `LayoutId` from `request_layout`, so gpui
//! hands them the child's own post-layout bounds at `prepaint`, which is
//! exactly the moment the geometry is final and nothing has been painted yet.
//!
//! Three properties are load-bearing and deliberate:
//!
//! * **`Element::id()` returns `None`.** Returning an id would push a
//!   `GlobalElementId`, which changes the element-state path of every
//!   descendant. The driver must not alter rendering, and a shifted state key
//!   is exactly that.
//! * **No taffy node of their own.** `request_layout` forwards the child's
//!   `LayoutId` rather than requesting one, so the layout tree with anchors is
//!   the layout tree without them.
//! * **The registry is read through `try_global`.** `global_mut` pushes a
//!   `NotifyGlobalObservers` effect; `try_global` does not, and the interior
//!   `RefCell` is enough to record through.

use std::cell::RefCell;
use std::rc::Rc;

use crowbar_ui::gpui::{
    AnyElement, App, Bounds, Element, ElementId, Global, GlobalElementId, InspectorElementId,
    InteractiveElement, IntoElement, LayoutId, Pixels, Refineable as _, SharedString, Style,
    StyledText, TextStyle, Window, px,
};

use crate::record::{self, RawAnchor, TextInput};
use crate::schema::{Snapshot, SnapshotError, SurfaceState};

/// Everything the anchors in one window recorded during the current frame.
///
/// A cheap handle: cloning it shares the same records, which is what lets an
/// anchor deep in the tree and the caller that serialises the snapshot both
/// hold one.
#[derive(Clone, Default)]
pub struct AnchorRegistry(Rc<RefCell<Vec<RawAnchor>>>);

impl Global for AnchorRegistry {}

/// Installs a registry on this app and hands back a handle to it.
///
/// Until this is called, every anchor is a pure pass-through that records
/// nothing — which is the behaviour a build that only *links* the driver should
/// have.
pub fn install(cx: &mut App) -> AnchorRegistry {
    let registry = AnchorRegistry::default();
    cx.set_global(registry.clone());
    registry
}

/// The registry installed on this app, if any.
#[must_use]
pub fn registry(cx: &App) -> Option<AnchorRegistry> {
    cx.try_global::<AnchorRegistry>().cloned()
}

impl AnchorRegistry {
    /// Forgets everything recorded so far.
    ///
    /// [`anchor_root`] calls this as it enters `prepaint`, which is what makes
    /// a snapshot one frame rather than an accumulation of them.
    pub fn clear(&self) {
        self.0.borrow_mut().clear();
    }

    /// The anchors recorded in the current frame, in the order `prepaint`
    /// reached them.
    #[must_use]
    pub fn records(&self) -> Vec<RawAnchor> {
        self.0.borrow().clone()
    }

    /// Builds a v1 snapshot from what has been recorded.
    ///
    /// # Errors
    ///
    /// [`SnapshotError::MissingRoot`] when the named root anchor was not one of
    /// them.
    pub fn snapshot(
        &self,
        surface: impl Into<String>,
        state: SurfaceState,
        root: &str,
    ) -> Result<Snapshot, SnapshotError> {
        Snapshot::build(surface, state, root, self.0.borrow().as_slice())
    }

    /// Records one anchor, replacing any earlier record with the same id.
    ///
    /// Replacement rather than a second entry because gpui may prepaint a
    /// subtree more than once in a frame; two rows for one id would make the
    /// snapshot depend on which one the differ happened to read.
    fn record(&self, raw: RawAnchor) {
        let mut records = self.0.borrow_mut();
        if let Some(existing) = records.iter_mut().find(|record| record.id == raw.id) {
            *existing = raw;
        } else {
            records.push(raw);
        }
    }
}

/// What a component **declares** about an anchor that neither extractor can
/// safely detect.
///
/// One value rather than a family of `anchor_*_sized` constructors, because the
/// two flags are independent and the third combination is real: the gate row's
/// `+n` count is content-sized *and* line-sized. Spelling that as
/// `anchor_text_content_and_line_sized` would be the point at which the API
/// starts hiding the thing it is supposed to make visible.
///
/// Every field defaults to `false`, so an ordinary `anchor(…)` declares
/// nothing and the declaration appears only where a component states it.
#[derive(Clone, Copy, Debug, Default, PartialEq, Eq)]
pub struct Declared {
    /// The box **sizes to its own text** (`native/oracle/ANCHORS.md` v1.5).
    ///
    /// The differ compares such a box's `bounds.w` against `ceil(reference)`,
    /// because gpui ceils a text run's max-content width and therefore cannot
    /// produce the fraction the DOM keeps. It is declared rather than worked
    /// out here, and the contract is emphatic about why: a heuristic here
    /// (`width: None` plus a text child) and the DOM side's heuristic
    /// (`width: auto` and not a stretched flex item) are both falsifiable by
    /// flex-grow, and two extractors each guessing wrong in different
    /// directions is invisible — it opens a blind spot or invents a delta and
    /// announces neither.
    pub content_sized: bool,
    /// The box height **is** the run's line box
    /// (`native/oracle/ANCHORS.md` v1.6).
    ///
    /// The differ then compares `bounds.h` against the reference's
    /// `font.line_height` rather than against its box: `WebKit` floors a line
    /// box to a whole logical pixel where gpui snaps it to the device grid, so
    /// the two boxes differ by up to a pixel while the line box behind them is
    /// the same number.
    ///
    /// Declared for a sharper reason than `content_sized`. "One text child and
    /// no explicit height" is exactly wrong for the gate row's badge, whose
    /// `sm:h-4` pins a 16px border box around a 13.33px line box — a detector
    /// would compare 16 against 13.33 and invent a delta out of nothing.
    pub line_sized: bool,
}

impl Declared {
    /// Declares nothing. The resting state, and what [`anchor`] uses.
    #[must_use]
    pub const fn nothing() -> Self {
        Self {
            content_sized: false,
            line_sized: false,
        }
    }

    /// [`Declared::content_sized`].
    #[must_use]
    pub const fn content_sized(mut self) -> Self {
        self.content_sized = true;
        self
    }

    /// [`Declared::line_sized`].
    #[must_use]
    pub const fn line_sized(mut self) -> Self {
        self.line_sized = true;
        self
    }
}

/// Opts an element into the snapshot under a stable semantic name.
///
/// The wrapped element is drawn exactly as it would have been.
#[must_use]
pub fn anchor<E>(id: impl Into<SharedString>, element: E) -> AnchoredBox
where
    E: InteractiveElement + IntoElement + 'static,
{
    AnchoredBox::wrap(id, element, false, Declared::nothing())
}

/// As [`anchor`], carrying the component's [`Declared`] claims about the box.
#[must_use]
pub fn anchor_declared<E>(
    id: impl Into<SharedString>,
    element: E,
    declared: Declared,
) -> AnchoredBox
where
    E: InteractiveElement + IntoElement + 'static,
{
    AnchoredBox::wrap(id, element, false, declared)
}

/// As [`anchor`], and additionally the frame boundary: all geometry in a
/// snapshot is relative to this anchor (§4), and reaching it clears whatever
/// the previous frame recorded.
#[must_use]
pub fn anchor_root<E>(id: impl Into<SharedString>, element: E) -> AnchoredBox
where
    E: InteractiveElement + IntoElement + 'static,
{
    AnchoredBox::wrap(id, element, true, Declared::nothing())
}

/// Renders a string *and* records it as a text anchor.
///
/// The element owns the string rather than being told what its child renders,
/// so `text` cannot drift from what is actually painted.
#[must_use]
pub fn anchor_text(id: impl Into<SharedString>, text: impl Into<SharedString>) -> AnchoredText {
    text_anchor(id, text, Declared::nothing())
}

/// As [`anchor_text`], carrying the component's [`Declared`] claims about the
/// run's box.
///
/// A bare `StyledText` is normally both: gpui measures it at exactly
/// `ceil(shaped advance)` wide — the ceil v1.5 models — and exactly one line
/// box tall, which is what v1.6 models.
#[must_use]
pub fn anchor_text_declared(
    id: impl Into<SharedString>,
    text: impl Into<SharedString>,
    declared: Declared,
) -> AnchoredText {
    text_anchor(id, text, declared)
}

fn text_anchor(
    id: impl Into<SharedString>,
    text: impl Into<SharedString>,
    declared: Declared,
) -> AnchoredText {
    let content: SharedString = text.into();
    AnchoredText {
        id: id.into(),
        child: StyledText::new(content.clone()).into_any_element(),
        content,
        declared,
    }
}

/// A box-shaped anchor: see [`anchor`].
pub struct AnchoredBox {
    id: SharedString,
    resets: bool,
    declared: Declared,
    style: Style,
    child: AnyElement,
}

impl AnchoredBox {
    fn wrap<E>(
        id: impl Into<SharedString>,
        mut element: E,
        resets: bool,
        declared: Declared,
    ) -> Self
    where
        E: InteractiveElement + IntoElement + 'static,
    {
        // The wrapped element's *base* style, resolved the way gpui resolves it
        // — `Style::default()` refined by the element's own refinement. That is
        // literally `Interactivity::compute_style_internal`'s first two lines.
        //
        // What it deliberately does not include is `hover`/`active`/`focus`
        // refinements, which gpui applies from runtime state that the element
        // does not carry until it has a hitbox. A snapshot of a hovered state
        // therefore has to be produced by a component that takes the state as a
        // prop, not by asking gpui to pretend the mouse is somewhere.
        let base = element.interactivity().base_style.clone();
        let mut style = Style::default();
        style.refine(&base);

        Self {
            id: id.into(),
            resets,
            declared,
            style,
            child: element.into_any_element(),
        }
    }
}

impl Element for AnchoredBox {
    type RequestLayoutState = ();
    type PrepaintState = ();

    fn id(&self) -> Option<ElementId> {
        None
    }

    fn source_location(&self) -> Option<&'static std::panic::Location<'static>> {
        None
    }

    fn request_layout(
        &mut self,
        _id: Option<&GlobalElementId>,
        _inspector_id: Option<&InspectorElementId>,
        window: &mut Window,
        cx: &mut App,
    ) -> (LayoutId, Self::RequestLayoutState) {
        (self.child.request_layout(window, cx), ())
    }

    fn prepaint(
        &mut self,
        _id: Option<&GlobalElementId>,
        _inspector_id: Option<&InspectorElementId>,
        bounds: Bounds<Pixels>,
        _request_layout: &mut Self::RequestLayoutState,
        window: &mut Window,
        cx: &mut App,
    ) {
        if let Some(registry) = registry(cx) {
            if self.resets {
                registry.clear();
            }
            registry.record(RawAnchor {
                // The component's claims, folded onto the style's facts. See
                // `record::box_facts`.
                content_sized: self.declared.content_sized,
                line_sized: self.declared.line_sized,
                ..record::box_facts(
                    self.id.clone(),
                    bounds,
                    window.content_mask().bounds,
                    &self.style,
                    window.rem_size(),
                )
            });
        }

        let _focus = self.child.prepaint(window, cx);
    }

    fn paint(
        &mut self,
        _id: Option<&GlobalElementId>,
        _inspector_id: Option<&InspectorElementId>,
        _bounds: Bounds<Pixels>,
        _request_layout: &mut Self::RequestLayoutState,
        _prepaint: &mut Self::PrepaintState,
        window: &mut Window,
        cx: &mut App,
    ) {
        self.child.paint(window, cx);
    }
}

impl IntoElement for AnchoredBox {
    type Element = Self;

    fn into_element(self) -> Self::Element {
        self
    }
}

/// A text anchor: see [`anchor_text`].
pub struct AnchoredText {
    id: SharedString,
    content: SharedString,
    declared: Declared,
    child: AnyElement,
}

impl Element for AnchoredText {
    type RequestLayoutState = ();
    type PrepaintState = ();

    fn id(&self) -> Option<ElementId> {
        None
    }

    fn source_location(&self) -> Option<&'static std::panic::Location<'static>> {
        None
    }

    fn request_layout(
        &mut self,
        _id: Option<&GlobalElementId>,
        _inspector_id: Option<&InspectorElementId>,
        window: &mut Window,
        cx: &mut App,
    ) -> (LayoutId, Self::RequestLayoutState) {
        (self.child.request_layout(window, cx), ())
    }

    fn prepaint(
        &mut self,
        _id: Option<&GlobalElementId>,
        _inspector_id: Option<&InspectorElementId>,
        bounds: Bounds<Pixels>,
        _request_layout: &mut Self::RequestLayoutState,
        window: &mut Window,
        cx: &mut App,
    ) {
        if let Some(registry) = registry(cx) {
            // Nothing between this element and the `StyledText` it wraps pushes
            // a text style, so what gpui's own text element resolves at this
            // point in the tree is exactly what `window.text_style()` says here.
            let style = window.text_style();
            let rem_size = window.rem_size();
            let font_size = style.font_size.to_pixels(rem_size);
            let line_height =
                window.pixel_snap(style.line_height.to_pixels(font_size.into(), rem_size));
            let shaped_width = shaped_width(window, &self.content, &style, font_size);

            registry.record(record::text_facts(&TextInput {
                id: self.id.clone(),
                bounds,
                clip: window.content_mask().bounds,
                style: &style,
                font_size,
                line_height,
                content: self.content.clone(),
                shaped_width,
                content_sized: self.declared.content_sized,
                line_sized: self.declared.line_sized,
            }));
        }

        let _focus = self.child.prepaint(window, cx);
    }

    fn paint(
        &mut self,
        _id: Option<&GlobalElementId>,
        _inspector_id: Option<&InspectorElementId>,
        _bounds: Bounds<Pixels>,
        _request_layout: &mut Self::RequestLayoutState,
        _prepaint: &mut Self::PrepaintState,
        window: &mut Window,
        cx: &mut App,
    ) {
        self.child.paint(window, cx);
    }
}

impl IntoElement for AnchoredText {
    type Element = Self;

    fn into_element(self) -> Self::Element {
        self
    }
}

/// The **unclipped** shaped advance width of the whole string.
///
/// This is the field `bounds` cannot substitute for: two implementations can
/// agree on the box and disagree on where the ellipsis lands, and only the
/// width the string *would* have occupied makes that visible.
///
/// It costs one line-layout cache entry per text anchor per frame when the
/// string is truncated (when it is not, the entry is the one gpui's own text
/// element already made, so this is a cache hit). The cache is keyed by content
/// and its contents do not feed back into layout, so the extra entry changes
/// what is retained and nothing that is drawn.
fn shaped_width(
    window: &Window,
    content: &SharedString,
    style: &TextStyle,
    font_size: Pixels,
) -> Pixels {
    let text_system = window.text_system().clone();

    if !content.contains('\n') {
        return text_system
            .shape_line(
                content.clone(),
                font_size,
                &[style.to_run(content.len())],
                None,
            )
            .width();
    }

    // `shape_line` is single-line only. A multi-line anchor's advance width is
    // its widest line, which is what a DOM `Range` over the same text node
    // would report too.
    let mut widest = px(0.0);
    for line in content.split('\n') {
        let width = text_system
            .shape_line(
                SharedString::from(line.to_owned()),
                font_size,
                &[style.to_run(line.len())],
                None,
            )
            .width();
        if width > widest {
            widest = width;
        }
    }
    widest
}

#[cfg(test)]
mod tests {
    use gpui::{
        Context, Empty, IntoElement, ParentElement as _, Render, Styled as _, TestAppContext,
        Window, div, px, rgb, size,
    };

    use super::{anchor, anchor_root, anchor_text, install, registry};
    use crate::record::Paint;
    use crate::schema::{Content, SurfaceState, Theme};

    /// A padded, offset, nested surface. The outer, *unanchored* div is what
    /// makes the root-relative arithmetic in `schema.rs` observable end to end
    /// rather than a tautology at the window origin — taffy ignores a margin on
    /// the root node, so the offset has to come from a parent's padding.
    fn a_surface() -> impl IntoElement {
        div().pl(px(25.0)).pt(px(40.0)).child(anchor_root(
            "root",
            div()
                .w(px(200.0))
                .h(px(60.0))
                .p(px(8.0))
                .rounded(px(6.0))
                .border_2()
                .border_color(rgb(0x003a_3f4b))
                .bg(rgb(0x0020_2734))
                .child(anchor(
                    "child",
                    div().w(px(40.0)).h(px(10.0)).bg(rgb(0x0058_a6ff)),
                )),
        ))
    }

    struct Surface;

    impl Render for Surface {
        fn render(&mut self, _window: &mut Window, _cx: &mut Context<Self>) -> impl IntoElement {
            a_surface()
        }
    }

    struct TextSurface {
        width: f32,
        truncate: bool,
    }

    impl Render for TextSurface {
        fn render(&mut self, _window: &mut Window, _cx: &mut Context<Self>) -> impl IntoElement {
            let row = div()
                .w(px(self.width))
                .overflow_hidden()
                .whitespace_nowrap()
                .child(anchor_text("label", "resolve-terminal-connection.ts"));

            anchor_root(
                "root",
                div().w(px(400.0)).child(if self.truncate {
                    row.text_ellipsis()
                } else {
                    row
                }),
            )
        }
    }

    /// Two anchors under one id, deliberately different sizes so which one
    /// survived is observable.
    struct CollidingSurface;

    impl Render for CollidingSurface {
        fn render(&mut self, _window: &mut Window, _cx: &mut Context<Self>) -> impl IntoElement {
            anchor_root(
                "root",
                div()
                    .flex()
                    .flex_col()
                    .child(anchor("twice", div().w(px(40.0)).h(px(10.0))))
                    .child(anchor("twice", div().w(px(60.0)).h(px(20.0)))),
            )
        }
    }

    struct MultiLineSurface;

    impl Render for MultiLineSurface {
        fn render(&mut self, _window: &mut Window, _cx: &mut Context<Self>) -> impl IntoElement {
            anchor_root(
                "root",
                div()
                    .flex()
                    .flex_col()
                    .child(anchor_text("multi", "one\ntwo-is-longer"))
                    .child(anchor_text("widest", "two-is-longer")),
            )
        }
    }

    #[gpui::test]
    fn an_anchor_records_its_post_layout_bounds(cx: &mut TestAppContext) {
        crate::leak_checked!(cx);
        let anchors = cx.update(install);
        let _window = cx.open_window(size(px(800.0), px(600.0)), |_, _| Surface);
        cx.run_until_parked();

        let records = anchors.records();

        assert_eq!(records.len(), 2);
        assert_eq!(records[0].id, "root");
        assert_eq!(records[0].bounds.origin, gpui::point(px(25.0), px(40.0)));
        assert_eq!(records[0].bounds.size, size(px(200.0), px(60.0)));
    }

    /// The wrapper contributes no taffy node of its own, so the child sits
    /// where the padding and the border put it and nowhere else.
    #[gpui::test]
    fn a_nested_anchor_lands_inside_its_parents_padding_and_border(cx: &mut TestAppContext) {
        crate::leak_checked!(cx);
        let anchors = cx.update(install);
        let _window = cx.open_window(size(px(800.0), px(600.0)), |_, _| Surface);
        cx.run_until_parked();

        let records = anchors.records();

        assert_eq!(records[1].id, "child");
        assert_eq!(records[1].bounds.origin, gpui::point(px(35.0), px(50.0)));
        assert_eq!(records[1].bounds.size, size(px(40.0), px(10.0)));
    }

    #[gpui::test]
    fn the_snapshot_is_relative_to_the_root_anchor(cx: &mut TestAppContext) {
        crate::leak_checked!(cx);
        let anchors = cx.update(install);
        let _window = cx.open_window(size(px(800.0), px(600.0)), |_, _| Surface);
        cx.run_until_parked();

        let snapshot = anchors
            .snapshot("driver-surface", a_state(), "root")
            .expect("the root anchor was recorded");

        assert_px(snapshot.anchors[0].bounds.x, 0.0);
        assert_px(snapshot.anchors[0].bounds.y, 0.0);
        assert_px(snapshot.anchors[1].bounds.x, 10.0);
        assert_px(snapshot.anchors[1].bounds.y, 10.0);
    }

    #[gpui::test]
    fn the_resolved_style_reaches_the_record(cx: &mut TestAppContext) {
        crate::leak_checked!(cx);
        let anchors = cx.update(install);
        let _window = cx.open_window(size(px(800.0), px(600.0)), |_, _| Surface);
        cx.run_until_parked();

        let records = anchors.records();

        assert_eq!(records[0].radius, px(6.0));
        assert_eq!(records[0].border_width, px(2.0));
        assert!(records[0].border_color.is_some());
        assert!(matches!(records[0].background, Paint::Solid(_)));
        assert!(records[0].visible);
    }

    #[gpui::test]
    fn a_text_anchor_records_the_full_string_and_its_unclipped_width(cx: &mut TestAppContext) {
        crate::leak_checked!(cx);
        let anchors = cx.update(install);
        let _window = cx.open_window(size(px(800.0), px(600.0)), |_, _| TextSurface {
            width: 400.0,
            truncate: false,
        });
        cx.run_until_parked();

        let records = anchors.records();
        let text = records[1]
            .text
            .as_ref()
            .expect("the text anchor paints text");

        assert_eq!(records[1].id, "label");
        assert_eq!(text.content, "resolve-terminal-connection.ts");
        assert!(text.width > px(0.0));
        assert!(!text.clipped);
        assert!(text.font.size > px(0.0));
        assert!(text.font.line_height > px(0.0));
    }

    /// The point of `text_width`: the box shrinks to the ellipsised string, so
    /// only the width the whole string *would* have taken can say where the
    /// truncation happened.
    #[gpui::test]
    fn a_truncated_text_anchor_reports_the_width_of_the_whole_string(cx: &mut TestAppContext) {
        crate::leak_checked!(cx);
        let anchors = cx.update(install);
        let _window = cx.open_window(size(px(800.0), px(600.0)), |_, _| TextSurface {
            width: 60.0,
            truncate: true,
        });
        cx.run_until_parked();

        let records = anchors.records();
        let record = &records[1];
        let text = record.text.as_ref().expect("the anchor paints text");

        assert_eq!(text.content, "resolve-terminal-connection.ts");
        assert!(text.clipped);
        assert!(text.width > record.bounds.size.width);
        assert!(record.bounds.size.width <= px(60.0));
    }

    #[gpui::test]
    fn the_root_anchor_clears_the_previous_frame(cx: &mut TestAppContext) {
        crate::leak_checked!(cx);
        let anchors = cx.update(install);
        let window = cx.open_window(size(px(800.0), px(600.0)), |_, _| Surface);
        cx.run_until_parked();
        assert_eq!(anchors.records().len(), 2);

        window
            .update(cx, |_, window, _| window.refresh())
            .expect("the window is open");
        cx.run_until_parked();

        assert_eq!(anchors.records().len(), 2);
    }

    /// A build that links the driver but never installs a registry has to draw
    /// identically and record nothing.
    #[gpui::test]
    fn anchors_are_a_pass_through_when_no_registry_is_installed(cx: &mut TestAppContext) {
        crate::leak_checked!(cx);
        let _window = cx.open_window(size(px(800.0), px(600.0)), |_, _| Surface);
        cx.run_until_parked();

        assert!(cx.update(|cx| registry(cx)).is_none());
    }

    #[gpui::test]
    fn an_empty_window_records_nothing_and_fails_to_snapshot(cx: &mut TestAppContext) {
        crate::leak_checked!(cx);
        let anchors = cx.update(install);
        let _window = cx.open_window(size(px(800.0), px(600.0)), |_, _| Empty);
        cx.run_until_parked();

        assert!(anchors.records().is_empty());
        assert!(
            anchors
                .snapshot("driver-surface", a_state(), "root")
                .is_err()
        );
    }

    /// Two rows for one id would make the snapshot depend on which one the
    /// differ happened to read, so the later record wins outright.
    #[gpui::test]
    fn a_repeated_anchor_id_replaces_rather_than_duplicates(cx: &mut TestAppContext) {
        crate::leak_checked!(cx);
        let anchors = cx.update(install);
        let _window = cx.open_window(size(px(800.0), px(600.0)), |_, _| CollidingSurface);
        cx.run_until_parked();

        let records = anchors.records();

        assert_eq!(records.len(), 2);
        assert_eq!(records[1].id, "twice");
        assert_eq!(records[1].bounds.size, size(px(60.0), px(20.0)));
    }

    /// `shape_line` is single-line only, so a string with a newline in it takes
    /// the other path: shape each line, keep the widest.
    #[gpui::test]
    fn a_multi_line_anchor_reports_the_width_of_its_widest_line(cx: &mut TestAppContext) {
        crate::leak_checked!(cx);
        let anchors = cx.update(install);
        let _window = cx.open_window(size(px(800.0), px(600.0)), |_, _| MultiLineSurface);
        cx.run_until_parked();

        let records = anchors.records();
        let multi = records[1].text.as_ref().expect("paints text");
        let widest = records[2].text.as_ref().expect("paints text");

        assert_eq!(multi.content, "one\ntwo-is-longer");
        assert_eq!(multi.width, widest.width);
    }

    /// See `schema.rs`: `px_of` rounds to a thousandth of a pixel, so this is
    /// exact equality spelled so `clippy::float_cmp` can see it is deliberate.
    #[track_caller]
    fn assert_px(actual: f64, expected: f64) {
        assert!(
            (actual - expected).abs() < 1e-9,
            "expected {expected}, got {actual}",
        );
    }

    fn a_state() -> SurfaceState {
        SurfaceState::new(800, Theme::Dark, Content::Normal, [])
    }
}
