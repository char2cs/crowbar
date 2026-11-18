//! Colour variants of a badge.

/// Colour treatment of a [`crate::Badge`].
#[derive(Debug, Clone, Copy, PartialEq, Eq, Default)]
#[non_exhaustive]
pub enum BadgeVariant {
    /// Solid primary.
    #[default]
    Primary,
    /// Solid destructive.
    Destructive,
    /// Tinted destructive.
    Error,
    /// Tinted info.
    Info,
    /// Bordered on the background.
    Outline,
    /// Solid secondary.
    Secondary,
    /// Tinted success.
    Success,
    /// Tinted warning.
    Warning,
}
