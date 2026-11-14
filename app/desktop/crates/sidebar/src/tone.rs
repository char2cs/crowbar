//! The colour of a menu entry.

/// Destructive entries are drawn in the destructive colour.
#[derive(Debug, Clone, Copy, PartialEq, Eq, Default)]
pub enum Tone {
    /// The foreground colour.
    #[default]
    Normal,
    /// The destructive colour.
    Destructive,
}
