//! How an open sidebar is placed relative to the content.

/// How an open sidebar sits beside or over the content.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum Presentation {
    /// Beside the content, taking its own width.
    Docked,
    /// Over the content, used when the container is narrow.
    Overlay,
}
