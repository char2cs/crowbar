//! How an open sidebar is placed relative to the content.

/// A docked sidebar owns a grid column; in a narrow window it floats over the content as a drawer.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum Presentation {
    /// A column beside the content.
    Docked,
    /// A drawer over the content with a scrim behind it.
    Overlay,
}
