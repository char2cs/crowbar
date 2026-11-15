//! How an open sidebar is placed relative to the content.

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub(crate) enum Presentation {
    Docked,
    Overlay,
}
