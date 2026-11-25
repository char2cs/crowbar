//! How far a floating layer reaches over its window.


#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub(crate) enum Reach {
    /// At the place its portal took in the window.
    Here,
    /// Over the whole window.
    Window,
    /// Over the whole window, painted in place so that it may hold floating elements of its
    /// own.
    Cover,
    /// At its portal's place like `Here`, painted in place, for the view that follows a
    /// dragged item.
    Lifted,
}
