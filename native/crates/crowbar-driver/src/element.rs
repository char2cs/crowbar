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

use std::cell::{Cell, RefCell};
use std::rc::Rc;

use crowbar_ui::gpui::{
    AnyElement, App, Bounds, Element, ElementId, Global, GlobalElementId, InspectorElementId,
    InteractiveElement, IntoElement, LayoutId, Pixels, Refineable as _, SharedString, Style,
    StyledText, TextStyle, Window, px,
};

use crate::record::{self, Opacity, RawAnchor, TextInput};
use crate::schema::{Snapshot, SnapshotError, SurfaceState};

/// Everything the anchors in one window recorded during the current frame,
/// plus the opacity in force at the point `prepaint` has reached.
///
/// A cheap handle: cloning it shares the same records, which is what lets an
/// anchor deep in the tree and the caller that serialises the snapshot both
/// hold one.
///
/// The opacity chain lives here rather than in the elements because gpui gives
/// an element no way to see its ancestors — see the crate docs' `visible`
/// bullet. `prepaint` descends in tree order, so an [`AnchoredBox`] that pushes
/// its own opacity before prepainting its child and restores it afterwards
/// reconstructs exactly the part of the chain the driver can see.
#[derive(Clone, Default)]
pub struct AnchorRegistry {
    records: Rc<RefCell<Vec<RawAnchor>>>,
    opacity: Rc<Cell<Opacity>>,
}

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
    ///
    /// The opacity chain is **not** reset with it. A translucent ancestor above
    /// the root anchor is not in the snapshot, but it is still an ancestor, and
    /// `visible` is a statement about what a user sees rather than about which
    /// rows the differ happens to read.
    pub fn clear(&self) {
        self.records.borrow_mut().clear();
    }

    /// The anchors recorded in the current frame, in the order `prepaint`
    /// first reached them.
    ///
    /// **Not deduplicated by id, except where two recordings agree.**
    /// [`AnchorRegistry::record`]'s docs cover the two cases: a repeat with
    /// identical content is folded into the one record already held, but a
    /// second, differing recording under the same id is kept alongside the
    /// first — [`Snapshot::build`]'s duplicate-id check (v1.8) is what refuses
    /// that, and every path that turns a frame into a snapshot goes through
    /// it. A caller that reads `records()` directly, as
    /// [`crate::frame::Settling`] does, is comparing raw frames rather than
    /// building a snapshot and is unaffected either way.
    #[must_use]
    pub fn records(&self) -> Vec<RawAnchor> {
        self.records.borrow().clone()
    }

    /// The opacity in force where `prepaint` currently is.
    fn opacity(&self) -> Opacity {
        self.opacity.get()
    }

    /// Descends into an element whose own declared opacity is `declared`,
    /// returning the chain to hand back to [`AnchorRegistry::leave`].
    ///
    /// Restoring rather than popping a stack, so that a subtree gpui prepaints
    /// twice in one frame — which it may — cannot leave the chain deeper than
    /// it found it.
    fn enter(&self, declared: Option<f32>) -> Opacity {
        let previous = self.opacity.get();
        self.opacity.set(previous.under(declared));
        previous
    }

    /// Undoes one [`AnchorRegistry::enter`].
    fn leave(&self, previous: Opacity) {
        self.opacity.set(previous);
    }

    /// Builds a v1 snapshot from what has been recorded.
    ///
    /// # Errors
    ///
    /// [`SnapshotError::DuplicateAnchor`] when some id was recorded more than
    /// once, or [`SnapshotError::MissingRoot`] when the named root anchor was
    /// not one of them.
    pub fn snapshot(
        &self,
        surface: impl Into<String>,
        state: SurfaceState,
        root: &str,
    ) -> Result<Snapshot, SnapshotError> {
        Snapshot::build(surface, state, root, self.records.borrow().as_slice())
    }

    /// Records one anchor.
    ///
    /// This is called from [`AnchoredBox::prepaint`] and
    /// [`AnchoredText::prepaint`] — `Element` callbacks that return `()`, not a
    /// `Result`, and that gpui calls from inside its own draw loop, where
    /// panicking would take the window down with it rather than fail one
    /// capture. Neither mechanism a caller would reach for — `?` or `panic!` —
    /// is reachable from here, so this function cannot itself refuse anything;
    /// it can only decide what to remember.
    ///
    /// # Same id, same content: folded away, not a conflict
    ///
    /// [`AnchorRegistry::clear`]'s own docs say `anchor_root` is "what makes a
    /// snapshot one frame" — which is only true of an anchor that calls
    /// [`anchor_root`]/[`anchor_root_declared`]. A **leaf, embeddable**
    /// component — `spinner`, `loading-spinner` — deliberately does not: its own
    /// root is a plain [`anchor`]/[`anchor_declared`] (`crowbar_ui::primitives::spinner`'s
    /// docs explain why — the same nested-root-wipes-the-ancestor
    /// hazard `inline_error.rs` documents for `Button`), so nothing ever calls
    /// `clear` while its window is open. `spinner`'s glyph rotates once a
    /// second, so gpui redraws it forever, and every one of those draws records
    /// the same id again with the same `bounds` — gpui rotates at **paint**
    /// time, so *layout* never moves. Refusing on a second recording,
    /// unconditionally, would refuse `spinner` and `loading-spinner` on their
    /// second draw, always — not a v1.8 violation, a surface with no frame
    /// boundary of its own being asked to have one anyway. So a new record that
    /// is `==` the most recent record already held under the same id is treated
    /// as another observation of the same anchor and does not extend the
    /// vector — which is also what keeps [`crate::frame::Settling`] able to
    /// converge on such a surface at all: it settles by comparing successive
    /// `records()` for equality, and a vector one entry longer on every draw
    /// would never repeat.
    ///
    /// # Same id, different content: kept, both of them
    ///
    /// If the most recent record under an id is **not** identical to the new
    /// one, the new one is pushed anyway rather than replacing it — this is the
    /// real collision `native/oracle/ANCHORS.md` v1.8 refuses: two distinct
    /// anchors sharing an id, which cannot be resolved here because
    /// *"taking the first would make that choice invisible, which is worse
    /// than the ambiguity"* (`oracleSelectDeclaredAnchors`, the DOM extractor's
    /// wording for the identical shape). An earlier version of this function
    /// replaced unconditionally, on the reasoning that gpui might prepaint a
    /// subtree more than once in a frame; that reasoning covered only the
    /// identical-content case above; it also silently discarded the *evidence*
    /// of a real, differing-content collision along with the collision itself.
    /// Keeping both is what lets [`crate::schema::Snapshot::build`] — the
    /// function that actually refuses it, because it is the one with a
    /// `Result` to refuse through, and every path that turns a frame into a
    /// snapshot (`AnchorRegistry::snapshot` and `crowbar-app`'s
    /// `row_snapshot::emit`) funnels through it — see the collision at all.
    fn record(&self, raw: RawAnchor) {
        let mut records = self.records.borrow_mut();
        let unchanged = records
            .iter()
            .rev()
            .find(|record| record.id == raw.id)
            .is_some_and(|existing| *existing == raw);
        if !unchanged {
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

/// As [`anchor_root`], carrying the component's [`Declared`] claims about the
/// box.
///
/// # Why the root needs this at all
///
/// Added independently by P3.19 and P3.20, which found the same hole from two
/// different surfaces: `keybinding` (P3.20) renders one `<kbd>` and nothing
/// else, so the frame boundary and the measured box are the same element;
/// `sidebar-empty` (P3.19) is a shrink-to-fit column inside a row flex parent,
/// so its used width **is** a text run's max-content width plus integral
/// padding. Both are surfaces whose **root is itself content-sized**.
///
/// Until then every root was a container — a row, a popup, a panel — whose
/// width came from its parent, so `anchor_root` hardcoding
/// [`Declared::nothing`] cost nothing and nobody noticed. It was still a hole,
/// and of the worst kind: a component that declared `content_sized` on its root
/// had the declaration **silently dropped in translation**. The DOM extractor
/// has no such hole — it reads `data-oracle-content-sized` off whichever
/// element carries the root id — so the reference said `content_sized: true`
/// and the native side said `false`, and the differ reported that as a
/// `ContentSizedMismatch` rather than as a missing feature: `bounds.w` compared
/// against the reference's fraction rather than against `ceil(reference.w)`, a
/// blind spot that reports nothing, which is exactly what
/// `native/oracle/ANCHORS.md` v1.5 says a mis-declaration does.
///
/// §4 zeroes a root's `x` and `y`, so only `w` and `h` are ever compared on one
/// — but those are precisely the two fields v1.5 and v1.6 correct.
#[must_use]
pub fn anchor_root_declared<E>(
    id: impl Into<SharedString>,
    element: E,
    declared: Declared,
) -> AnchoredBox
where
    E: InteractiveElement + IntoElement + 'static,
{
    AnchoredBox::wrap(id, element, true, declared)
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
        // `enter` **before** recording, so that this element's own opacity
        // counts against its own `visible` and not only its children's — which
        // is what `oracleIsVisible` does when it tests `el` before walking up
        // from `el.parentElement`.
        let descent = registry(cx).map(|registry| {
            if self.resets {
                registry.clear();
            }
            let previous = registry.enter(self.style.opacity);
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
                    registry.opacity(),
                )
            });
            (registry, previous)
        });

        let _focus = self.child.prepaint(window, cx);

        if let Some((registry, previous)) = descent {
            registry.leave(previous);
        }
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
                // A `StyledText` has no opacity of its own to fold in, so this
                // run is exactly as opaque as the box that holds it.
                opacity: registry.opacity(),
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

    use super::{
        AnchorRegistry, Declared, anchor, anchor_declared, anchor_root, anchor_root_declared,
        anchor_text, install, registry,
    };
    use crate::record::{Paint, RawAnchor};
    use crate::schema::{Content, SurfaceState, Theme};

    /// A bare box record, for testing [`AnchorRegistry::record`] directly
    /// without a window.
    fn a_box_record(id: &str, x: f32, y: f32, w: f32, h: f32) -> RawAnchor {
        RawAnchor {
            id: id.into(),
            bounds: gpui::bounds(gpui::point(px(x), px(y)), size(px(w), px(h))),
            background: Paint::None,
            text: None,
            visible: true,
            radius: px(0.0),
            border_width: px(0.0),
            border_color: None,
            content_sized: false,
            line_sized: false,
        }
    }

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

    /// A root that declares `content_sized` around a child that declares
    /// `line_sized`, so the two flags cannot be confused for one another.
    struct Declaring;

    impl Render for Declaring {
        fn render(&mut self, _window: &mut Window, _cx: &mut Context<Self>) -> impl IntoElement {
            anchor_root_declared(
                "root",
                div().child(anchor_declared(
                    "child",
                    div().w(px(10.0)).h(px(10.0)),
                    Declared::nothing().line_sized(),
                )),
                Declared::nothing().content_sized(),
            )
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

    /// Two anchors under one id, deliberately different sizes so which record
    /// is which is observable.
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

    /// Three anchors under one id, for [`a_repeated_anchor_id_names_how_many_times_it_was_seen`].
    struct TriplyCollidingSurface;

    impl Render for TriplyCollidingSurface {
        fn render(&mut self, _window: &mut Window, _cx: &mut Context<Self>) -> impl IntoElement {
            anchor_root(
                "root",
                div()
                    .flex()
                    .flex_col()
                    .child(anchor("thrice", div().w(px(10.0)).h(px(10.0))))
                    .child(anchor("thrice", div().w(px(20.0)).h(px(10.0))))
                    .child(anchor("thrice", div().w(px(30.0)).h(px(10.0)))),
            )
        }
    }

    /// `corpus/001`'s sequence, in the smallest tree that carries it: a layer
    /// at `opacity-0` holding a box anchor, a text anchor and a nested anchor
    /// two levels down, plus a sibling outside the layer that must stay
    /// visible.
    ///
    /// The layer is the **root anchor** rather than an unanchored ancestor of
    /// it, which is the placement the crate docs' `visible` bullet requires and
    /// costs nothing: opacity changes no geometry, and every bound in a
    /// snapshot is relative to the root either way.
    struct TranslucentSurface {
        opacity: f32,
    }

    impl Render for TranslucentSurface {
        fn render(&mut self, _window: &mut Window, _cx: &mut Context<Self>) -> impl IntoElement {
            div().flex().flex_col().child(anchor_root(
                "root",
                div()
                    .opacity(self.opacity)
                    .child(anchor(
                        "panel",
                        div()
                            .w(px(40.0))
                            .h(px(10.0))
                            .child(anchor("nested", div().w(px(20.0)).h(px(5.0)))),
                    ))
                    .child(anchor_text("label", "carousel")),
            ))
        }
    }

    /// The other half of the same question: a *sibling* of the transparent
    /// layer is untouched by it, so the chain has to be restored on the way
    /// back up and not merely accumulated on the way down.
    struct SiblingSurface;

    impl Render for SiblingSurface {
        fn render(&mut self, _window: &mut Window, _cx: &mut Context<Self>) -> impl IntoElement {
            anchor_root(
                "root",
                div()
                    .flex()
                    .flex_col()
                    .child(anchor(
                        "hidden-layer",
                        div()
                            .opacity(0.0)
                            .child(anchor("under-layer", div().w(px(40.0)).h(px(10.0)))),
                    ))
                    .child(anchor("sibling", div().w(px(40.0)).h(px(10.0)))),
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

    /// **A root anchor carries its declarations**, which it could not before
    /// P3.19 — [`anchor_root`] hard-coded [`Declared::nothing`] and there was no
    /// other way in.
    ///
    /// The hole was invisible from inside this crate and loud from outside it:
    /// the DOM extractor reads `data-oracle-content-sized` off whichever element
    /// carries the root id, so a surface whose root shrink-wraps emitted `true`
    /// on the reference and `false` here, and the differ reports that as a
    /// `ContentSizedMismatch` on every cell.
    ///
    /// The control is the second half — [`anchor_root`] still declares nothing,
    /// so this test cannot pass by both spellings having become the same thing.
    #[gpui::test]
    fn a_root_anchor_carries_the_declarations_it_is_given(cx: &mut TestAppContext) {
        crate::leak_checked!(cx);
        let anchors = cx.update(install);
        let _window = cx.open_window(size(px(800.0), px(600.0)), |_, _| Declaring);
        cx.run_until_parked();

        let records = anchors.records();
        let root = records
            .iter()
            .find(|record| record.id == "root")
            .expect("the root was recorded");
        assert!(root.content_sized, "{root:?}");
        assert!(!root.line_sized, "{root:?}");
        let child = records
            .iter()
            .find(|record| record.id == "child")
            .expect("the child was recorded");
        assert!(child.line_sized, "{child:?}");
        assert!(!child.content_sized, "{child:?}");
    }

    /// The control for the test above: the undeclared spelling still declares
    /// nothing, so a root that *should* be plain cannot be silently promoted.
    #[gpui::test]
    fn a_plain_root_anchor_declares_nothing(cx: &mut TestAppContext) {
        crate::leak_checked!(cx);
        let anchors = cx.update(install);
        let _window = cx.open_window(size(px(800.0), px(600.0)), |_, _| Surface);
        cx.run_until_parked();

        let records = anchors.records();
        assert!(!records[0].content_sized, "{:?}", records[0]);
        assert!(!records[0].line_sized, "{:?}", records[0]);
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

    /// **The discovery that shaped this item's design.** `spinner` and
    /// `loading-spinner` record their own root through a plain
    /// [`anchor`]/[`anchor_declared`], not [`anchor_root`] — see `record`'s own
    /// docs for why — so nothing ever clears their registry between draws, and
    /// `spinner`'s glyph redraws once a second forever. A `record` that treated
    /// every repeat as a conflict, unconditionally, refused both surfaces on
    /// their second draw, always: this reproduces that in miniature, no window
    /// required, directly against the method the redraw goes through.
    #[test]
    fn recording_the_identical_content_twice_does_not_grow_the_frame() {
        let registry = AnchorRegistry::default();
        registry.record(a_box_record("spinner", 0.0, 0.0, 16.0, 16.0));
        registry.record(a_box_record("spinner", 0.0, 0.0, 16.0, 16.0));

        assert_eq!(registry.records().len(), 1);
    }

    /// **The control for the test above.** Without it, "an identical repeat is
    /// folded away" would also pass on a `record` that folds away *every*
    /// repeat regardless of content — which is the old, replace-unconditionally
    /// behaviour this item removed, and precisely what let `CollidingSurface`
    /// (two boxes, one id, different sizes) go unnoticed. A repeat under the
    /// same id with different bounds is kept as a second row.
    #[test]
    fn recording_different_content_under_the_same_id_keeps_both() {
        let registry = AnchorRegistry::default();
        registry.record(a_box_record("twice", 0.0, 0.0, 40.0, 10.0));
        registry.record(a_box_record("twice", 0.0, 10.0, 60.0, 20.0));

        assert_eq!(registry.records().len(), 2);
    }

    /// However many times an *unchanged* value is re-observed, the frame stays
    /// at one record — not "the first two collapse and the third does not".
    #[test]
    fn many_identical_observations_still_fold_to_one() {
        let registry = AnchorRegistry::default();
        for _ in 0..300 {
            registry.record(a_box_record("spinner", 0.0, 0.0, 16.0, 16.0));
        }

        assert_eq!(registry.records().len(), 1);
    }

    /// **`record` keeps both anchors rather than picking one.** The evidence
    /// of the collision — two distinct boxes, both claiming `twice` — has to
    /// survive as far as [`crate::schema::Snapshot::build`], which is the
    /// function that actually refuses it (see the next test); if `record`
    /// silently replaced the first with the second, as it once did, there
    /// would be nothing left downstream to detect.
    #[gpui::test]
    fn a_repeated_anchor_id_is_recorded_as_two_rows_not_one(cx: &mut TestAppContext) {
        crate::leak_checked!(cx);
        let anchors = cx.update(install);
        let _window = cx.open_window(size(px(800.0), px(600.0)), |_, _| CollidingSurface);
        cx.run_until_parked();

        let records = anchors.records();

        assert_eq!(records.len(), 3, "{records:?}");
        let twice: Vec<_> = records.iter().filter(|r| r.id == "twice").collect();
        assert_eq!(twice.len(), 2, "{records:?}");
        assert_eq!(twice[0].bounds.size, size(px(40.0), px(10.0)));
        assert_eq!(twice[1].bounds.size, size(px(60.0), px(20.0)));
    }

    /// **The refusal itself**, taken at the boundary that can actually return
    /// one: building a snapshot from a frame that recorded `twice` twice
    /// fails by name rather than silently picking a record, matching
    /// `oracleSelectDeclaredAnchors`'s refusal for the identical shape on the
    /// DOM side ("two elements carry one declared id → throws").
    ///
    /// This is the mutation target for P3.42: reverting `AnchorRegistry::record`
    /// to its old replace-on-collision behaviour makes `records()` report a
    /// single `twice` entry again, `Snapshot::build` never sees a repeat, and
    /// this assertion fails with "the frame recorded a collision" — see the
    /// worker's report for the exact diff and the failing output it produces.
    #[gpui::test]
    fn a_repeated_anchor_id_is_refused_when_the_snapshot_is_built(cx: &mut TestAppContext) {
        crate::leak_checked!(cx);
        let anchors = cx.update(install);
        let _window = cx.open_window(size(px(800.0), px(600.0)), |_, _| CollidingSurface);
        cx.run_until_parked();

        let error = anchors
            .snapshot("driver-surface", a_state(), "root")
            .expect_err("the frame recorded a collision");

        assert_eq!(
            error,
            crate::schema::SnapshotError::DuplicateAnchor {
                id: "twice".to_owned(),
                count: 2,
            },
        );
        assert!(error.to_string().contains("\"twice\""), "{error}");
    }

    /// However many times an id repeats, the count in the refusal is the real
    /// count and not a hard-coded "twice" — otherwise this would be a guard
    /// that happens to work on exactly the fixture it was written against.
    #[gpui::test]
    fn a_repeated_anchor_id_names_how_many_times_it_was_seen(cx: &mut TestAppContext) {
        crate::leak_checked!(cx);
        let anchors = cx.update(install);
        let _window = cx.open_window(size(px(800.0), px(600.0)), |_, _| TriplyCollidingSurface);
        cx.run_until_parked();

        let error = anchors
            .snapshot("driver-surface", a_state(), "root")
            .expect_err("the frame recorded a collision");

        assert_eq!(
            error,
            crate::schema::SnapshotError::DuplicateAnchor {
                id: "thrice".to_owned(),
                count: 3,
            },
        );
    }

    /// **The control.** Without it, "a colliding surface is refused" would
    /// also pass on a `snapshot` that refuses every frame unconditionally —
    /// the shape of vacuous guard this project has already shipped eight of.
    /// The ordinary two-anchor surface from the very first test in this
    /// module, which shares no id between its two anchors, still snapshots
    /// cleanly.
    #[gpui::test]
    fn a_surface_with_no_repeated_id_is_not_refused(cx: &mut TestAppContext) {
        crate::leak_checked!(cx);
        let anchors = cx.update(install);
        let _window = cx.open_window(size(px(800.0), px(600.0)), |_, _| Surface);
        cx.run_until_parked();

        let snapshot = anchors
            .snapshot("driver-surface", a_state(), "root")
            .expect("root and child use distinct ids");
        assert_eq!(snapshot.anchors.len(), 2);
    }

    /// `corpus/001`, driven: **every** anchor under a zero-opacity layer
    /// reports `visible: false`, the way `oracleIsVisible` has reported it all
    /// along.
    ///
    /// The control is the same tree at `opacity(1.0)`, in the same test: without
    /// it, "every anchor is invisible" would also pass on a driver that had
    /// broken `visible` outright, which is the shape of vacuous guard this
    /// codebase has produced before.
    #[gpui::test]
    fn every_anchor_under_a_zero_opacity_layer_is_invisible(cx: &mut TestAppContext) {
        crate::leak_checked!(cx);
        let anchors = cx.update(install);
        let _window = cx.open_window(size(px(800.0), px(600.0)), |_, _| TranslucentSurface {
            opacity: 0.0,
        });
        cx.run_until_parked();

        let records = anchors.records();

        assert_eq!(
            records.iter().map(|r| r.id.as_ref()).collect::<Vec<_>>(),
            ["root", "panel", "nested", "label"],
        );
        for record in &records {
            assert!(!record.visible, "{}", record.id);
            // Nothing else moved: the boxes are still laid out and measured.
            assert!(record.bounds.size.width > px(0.0), "{}", record.id);
        }
    }

    /// The control for the test above, and the one that makes it non-vacuous:
    /// the identical tree at full opacity reports every anchor visible.
    #[gpui::test]
    fn the_same_tree_at_full_opacity_is_visible_throughout(cx: &mut TestAppContext) {
        crate::leak_checked!(cx);
        let anchors = cx.update(install);
        let _window = cx.open_window(size(px(800.0), px(600.0)), |_, _| TranslucentSurface {
            opacity: 1.0,
        });
        cx.run_until_parked();

        for record in &anchors.records() {
            assert!(record.visible, "{}", record.id);
        }
    }

    /// The chain is restored on the way back up: a sibling of a transparent
    /// layer is visible, and so is the root above both. A driver that only ever
    /// accumulated would report the sibling invisible and pass every assertion
    /// in the two tests above.
    #[gpui::test]
    fn a_sibling_of_a_transparent_layer_keeps_its_own_opacity(cx: &mut TestAppContext) {
        crate::leak_checked!(cx);
        let anchors = cx.update(install);
        let _window = cx.open_window(size(px(800.0), px(600.0)), |_, _| SiblingSurface);
        cx.run_until_parked();

        let records = anchors.records();
        let visible = |id: &str| {
            records
                .iter()
                .find(|record| record.id == id)
                .unwrap_or_else(|| panic!("{id} was recorded"))
                .visible
        };

        assert!(visible("root"));
        assert!(!visible("hidden-layer"));
        assert!(!visible("under-layer"));
        assert!(visible("sibling"));
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
