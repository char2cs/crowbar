//! Which way the arrow keys move through a menu.

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub(crate) enum MenuStep {
    Next,
    Previous,
}
