//! How a component opts its elements into an oracle snapshot without depending
//! on the oracle.
//!
//! `crowbar-driver` — which owns the extractor — depends on **this** crate
//! (`crowbar_ui::gpui` is where it gets the framework from), so `crowbar-ui`
//! cannot depend on it back. A component that wants to be measurable therefore
//! cannot call `crowbar_driver::anchor` directly; it takes an [`AnchorSink`] and
//! the binary decides which one to hand it.
//!
//! Two implementations exist:
//!
//! * [`Unanchored`], here — the shipping path. Every method is the identity,
//!   and the elements it returns are byte-for-byte the ones the driver's
//!   wrappers would have wrapped, so **the layout is the same either way**.
//!   That is the property the whole arrangement exists for: an oracle that
//!   measures a differently-shaped tree measures nothing.
//! * the driver-backed one in `crowbar-app`, behind `--features driver`.
//!
//! [`Unanchored::text`] returns a [`StyledText`] rather than
//! `div().child(content)` on purpose: `crowbar_driver::anchor_text` renders a
//! `StyledText`, and only rendering the same element on both paths keeps the
//! shipping build and the measured one identical by construction rather than by
//! inspection. The same reasoning applies to [`Unanchored::boxed_text`].

use gpui::{AnyElement, Div, IntoElement as _, ParentElement as _, SharedString, StyledText};

/// An anchor id, plus the things about the anchor the *component* knows and
/// neither extractor can safely work out: whether the box sizes to its own
/// text, and whether its height *is* its own line box.
///
/// `native/oracle/ANCHORS.md` v1.5 and v1.6 make both **declared** properties,
/// and this type is why a declaration cannot be forgotten in one build and
/// remembered in another: the same value flows to the shipping sink, which
/// ignores it, and to the driver sink, which records it.
///
/// `From<&'static str>` and `From<SharedString>` keep every ordinary call site
/// reading exactly as it did — `anchors.boxed(ID_ICON.into(), …)` — so a
/// declaration appears only where it is true. The two builders chain, because
/// the third combination is real: the row's `+n` count is both.
#[derive(Clone, Debug, PartialEq, Eq)]
pub struct AnchorId {
    /// The stable semantic name, identical on both sides of the differ.
    pub id: SharedString,
    /// v1.5: this anchor's box sizes to its own text, so the differ compares
    /// `bounds.w` against `ceil(reference)`.
    pub content_sized: bool,
    /// v1.6: this anchor's box height is its own line box, so the differ
    /// compares `bounds.h` against the reference's `font.line_height` rather
    /// than against its box.
    ///
    /// **Only ever true on an anchor that paints text.** The differ refuses a
    /// snapshot that declares it without a `font`, which is the loud half of
    /// the same rule.
    pub line_sized: bool,
}

impl AnchorId {
    /// A plain anchor, declaring nothing. Equivalent to `id.into()`.
    #[must_use]
    pub fn new(id: impl Into<SharedString>) -> Self {
        Self {
            id: id.into(),
            content_sized: false,
            line_sized: false,
        }
    }

    /// Declares [`AnchorId::content_sized`] (v1.5).
    #[must_use]
    pub fn content_sized(mut self) -> Self {
        self.content_sized = true;
        self
    }

    /// Declares [`AnchorId::line_sized`] (v1.6).
    #[must_use]
    pub fn line_sized(mut self) -> Self {
        self.line_sized = true;
        self
    }
}

impl From<&'static str> for AnchorId {
    fn from(id: &'static str) -> Self {
        Self::new(SharedString::new_static(id))
    }
}

impl From<SharedString> for AnchorId {
    fn from(id: SharedString) -> Self {
        Self::new(id)
    }
}

/// A sink for the anchor ids a measurable component carries.
///
/// Object-safe on purpose: a component takes `&dyn AnchorSink` so that one
/// render path serves both the shipping build and the measured one.
pub trait AnchorSink {
    /// The frame boundary: every other anchor's geometry is reported relative
    /// to this one, and reaching it starts a new frame's worth of records.
    fn root(&self, id: AnchorId, element: Div) -> AnyElement;

    /// A box-shaped anchor — bounds, background, radius, border.
    fn boxed(&self, id: AnchorId, element: Div) -> AnyElement;

    /// A text-painting anchor — bounds, colour, the string and its unclipped
    /// shaped width.
    fn text(&self, id: AnchorId, content: SharedString) -> AnyElement;

    /// An anchor that is **both** a painted box and a text run, reported under
    /// one id.
    ///
    /// `native/oracle/ANCHORS.md` §3 has always permitted this — the field
    /// table's "required: if it paints text" group sits alongside `bg`,
    /// `radius` and `border`, not instead of them, and the DOM extractor emits
    /// both halves for any element that has both. Splitting it was a limitation
    /// of [`AnchorSink::boxed`] and [`AnchorSink::text`], and the first surface
    /// to need it — the `uncommitted` badge, a tinted rounded box *containing*
    /// a string — produced five `FieldPresence` deltas on one anchor because of
    /// it.
    ///
    /// The `content` is owned by the sink rather than being passed as a child,
    /// for the same reason [`AnchorSink::text`] owns its string: what is
    /// recorded cannot then drift from what is painted.
    fn boxed_text(&self, id: AnchorId, element: Div, content: SharedString) -> AnyElement;
}

/// The shipping sink: renders exactly what it was given and records nothing.
#[derive(Clone, Copy, Debug, Default, PartialEq, Eq)]
pub struct Unanchored;

impl AnchorSink for Unanchored {
    fn root(&self, _id: AnchorId, element: Div) -> AnyElement {
        element.into_any_element()
    }

    fn boxed(&self, _id: AnchorId, element: Div) -> AnyElement {
        element.into_any_element()
    }

    fn text(&self, _id: AnchorId, content: SharedString) -> AnyElement {
        StyledText::new(content).into_any_element()
    }

    fn boxed_text(&self, _id: AnchorId, element: Div, content: SharedString) -> AnyElement {
        element.child(StyledText::new(content)).into_any_element()
    }
}

#[cfg(test)]
mod tests {
    use super::AnchorId;

    /// The declarations are opt-in: an ordinary `.into()` call site cannot
    /// accidentally claim anything, and the ones that do say so say it in the
    /// component, where it is reviewable.
    #[test]
    fn an_ordinary_anchor_id_declares_nothing() {
        for plain in [AnchorId::from("git-row-icon"), AnchorId::new("git-row-icon")] {
            assert!(!plain.content_sized);
            assert!(!plain.line_sized);
        }
        assert_eq!(AnchorId::from("a"), AnchorId::new("a"));
        assert_ne!(AnchorId::from("a"), AnchorId::new("a").content_sized());
        assert_ne!(AnchorId::from("a"), AnchorId::new("a").line_sized());
    }

    /// The two are independent, and they chain — which they have to, because
    /// the row's `+n` count is both: a bare run whose box is the run's advance
    /// width *and* the run's line box.
    #[test]
    fn the_two_declarations_are_independent_and_compose() {
        let content = AnchorId::new("git-row-badge").content_sized();
        assert!(content.content_sized);
        assert!(!content.line_sized);

        let line = AnchorId::new("git-row-name").line_sized();
        assert!(line.line_sized);
        assert!(!line.content_sized);

        let both = AnchorId::new("git-row-added").content_sized().line_sized();
        assert!(both.content_sized && both.line_sized);
        // Order does not matter, and the id is untouched either way.
        assert_eq!(
            both,
            AnchorId::new("git-row-added").line_sized().content_sized()
        );
        assert_eq!(both.id, "git-row-added");
    }

    #[test]
    fn a_shared_string_id_converts_the_same_way() {
        let id = gpui::SharedString::from("git-row-guide-0".to_owned());
        let anchor = AnchorId::from(id.clone());
        assert_eq!(anchor.id, id);
        assert!(!anchor.content_sized);
        assert!(!anchor.line_sized);
    }
}
