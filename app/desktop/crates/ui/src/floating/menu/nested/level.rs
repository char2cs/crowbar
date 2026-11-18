//! Whether a menu is the root popup or opened from a submenu row.

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub(crate) enum Level {
    Root,
    Nested,
}
