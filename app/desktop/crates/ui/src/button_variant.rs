//! Visual variants of a button.

/// Colour treatment of a [`crate::Button`] or [`crate::IconButton`].
#[derive(Debug, Clone, Copy, PartialEq, Eq, Default)]
#[non_exhaustive]
pub enum ButtonVariant {
    /// Filled with the primary colour.
    #[default]
    Primary,
    /// Filled with the destructive colour.
    Destructive,
    /// Bordered on the popover surface.
    Outline,
    /// Filled with the secondary colour.
    Secondary,
    /// Transparent until hovered.
    Ghost,
}
