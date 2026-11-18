//! Phosphor stroke weights the shell uses.

/// A Phosphor weight.
#[derive(Debug, Clone, Copy, PartialEq, Eq, Hash, Default)]
#[non_exhaustive]
pub enum IconWeight {
    /// Default outline weight.
    #[default]
    Regular,
    /// Heavier outline used for row actions.
    Bold,
    /// Solid glyph.
    Fill,
    /// Outline with a 20% tinted body.
    Duotone,
}
