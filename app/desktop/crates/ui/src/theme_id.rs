//! The two palettes the shell ships.

/// Identifier of a colour theme (`data-theme` in the React app).
#[derive(Debug, Clone, Copy, PartialEq, Eq, Hash, Default)]
#[non_exhaustive]
pub enum ThemeId {
    /// The default theme: olive accent, dark chrome around a light canvas.
    #[default]
    Crowbar,
    /// The unthemed base palette with neutral chrome.
    Frosted,
}
