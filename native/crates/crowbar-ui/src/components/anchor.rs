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

/// A sink for the anchor ids a measurable component carries.
///
/// Object-safe on purpose: a component takes `&dyn AnchorSink` so that one
/// render path serves both the shipping build and the measured one.
pub trait AnchorSink {
    /// The frame boundary: every other anchor's geometry is reported relative
    /// to this one, and reaching it starts a new frame's worth of records.
    fn root(&self, id: SharedString, element: Div) -> AnyElement;

    /// A box-shaped anchor — bounds, background, radius, border.
    fn boxed(&self, id: SharedString, element: Div) -> AnyElement;

    /// A text-painting anchor — bounds, colour, the string and its unclipped
    /// shaped width.
    fn text(&self, id: SharedString, content: SharedString) -> AnyElement;

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
    fn boxed_text(&self, id: SharedString, element: Div, content: SharedString) -> AnyElement;
}

/// The shipping sink: renders exactly what it was given and records nothing.
#[derive(Clone, Copy, Debug, Default, PartialEq, Eq)]
pub struct Unanchored;

impl AnchorSink for Unanchored {
    fn root(&self, _id: SharedString, element: Div) -> AnyElement {
        element.into_any_element()
    }

    fn boxed(&self, _id: SharedString, element: Div) -> AnyElement {
        element.into_any_element()
    }

    fn text(&self, _id: SharedString, content: SharedString) -> AnyElement {
        StyledText::new(content).into_any_element()
    }

    fn boxed_text(&self, _id: SharedString, element: Div, content: SharedString) -> AnyElement {
        element.child(StyledText::new(content)).into_any_element()
    }
}
