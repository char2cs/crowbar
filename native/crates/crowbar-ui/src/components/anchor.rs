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
//! `StyledText`, and a `Text` element and a `StyledText` do not request layout
//! identically.

use gpui::{AnyElement, Div, IntoElement as _, SharedString, StyledText};

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
}
