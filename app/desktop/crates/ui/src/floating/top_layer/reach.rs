//! How far a floating layer reaches over the owner window.

use gpui::{Bounds, Pixels};

#[derive(Debug, Clone, Copy, PartialEq)]
pub(crate) enum Reach {
    /// At the place its portal took in the owner window, in owner coordinates.
    Here(Bounds<Pixels>),
    /// Over the whole owner window.
    Window,
    /// Over the whole owner window, painted in place when there is no top layer, so that it may
    /// hold floating elements of its own.
    Cover,
}
