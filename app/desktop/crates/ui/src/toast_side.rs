//! Which edge a toast column hugs, and so which way a swipe dismisses.

/// Left or right edge of the window.
#[derive(Debug, Clone, Copy, PartialEq, Eq, Default)]
#[non_exhaustive]
pub enum ToastSide {
    /// The sidebar sits on the left.
    #[default]
    Left,
    /// The sidebar sits on the right.
    Right,
}
