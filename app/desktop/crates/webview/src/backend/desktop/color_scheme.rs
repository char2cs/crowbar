//! The colour scheme the desktop's own widgets draw in.

/// The colour scheme of the desktop's own menus, dialogs and choosers.
#[derive(Debug, Clone, Copy, PartialEq, Eq, Hash)]
pub enum ColorScheme {
    /// Dark text on light surfaces.
    Light,
    /// Light text on dark surfaces.
    Dark,
}
