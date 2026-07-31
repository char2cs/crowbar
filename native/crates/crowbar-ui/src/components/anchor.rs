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

/// An anchor id, plus the one thing about the anchor the *component* knows and
/// neither extractor can safely work out: whether the box sizes to its own text.
///
/// `native/oracle/ANCHORS.md` v1.5 makes `content_sized` a **declared**
/// property, and this type is why the declaration cannot be forgotten in one
/// build and remembered in another: the same value flows to the shipping sink,
/// which ignores it, and to the driver sink, which records it.
///
/// `From<&'static str>` and `From<SharedString>` keep every ordinary call site
/// reading exactly as it did — `anchors.boxed(ID_ICON.into(), …)` — so the
/// declaration appears only where it is true.
#[derive(Clone, Debug, PartialEq, Eq)]
pub struct AnchorId {
    /// The stable semantic name, identical on both sides of the differ.
    pub id: SharedString,
    /// v1.5: this anchor's box sizes to its own text, so the differ compares
    /// `bounds.w` against `ceil(reference)`.
    pub content_sized: bool,
}

impl AnchorId {
    /// A plain anchor. Equivalent to `id.into()`.
    #[must_use]
    pub fn new(id: impl Into<SharedString>) -> Self {
        Self {
            id: id.into(),
            content_sized: false,
        }
    }

    /// An anchor whose box sizes to its own text (v1.5).
    #[must_use]
    pub fn content_sized(id: impl Into<SharedString>) -> Self {
        Self {
            id: id.into(),
            content_sized: true,
        }
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

    /// The declaration is opt-in: an ordinary `.into()` call site cannot
    /// accidentally claim a box is content-sized, and the one that does say so
    /// says it in the component, where it is reviewable.
    #[test]
    fn an_ordinary_anchor_id_is_not_content_sized() {
        assert!(!AnchorId::from("git-row-name").content_sized);
        assert!(!AnchorId::new("git-row-name").content_sized);
        assert!(AnchorId::content_sized("git-row-badge").content_sized);
        assert_eq!(AnchorId::from("a"), AnchorId::new("a"));
        assert_ne!(AnchorId::from("a"), AnchorId::content_sized("a"));
    }

    #[test]
    fn a_shared_string_id_converts_the_same_way() {
        let id = gpui::SharedString::from("git-row-guide-0".to_owned());
        let anchor = AnchorId::from(id.clone());
        assert_eq!(anchor.id, id);
        assert!(!anchor.content_sized);
    }
}
