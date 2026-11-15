//! Underline variants a cell can carry.

/// How a cell is underlined.
#[derive(Debug, Clone, Copy, Default, PartialEq, Eq, Hash)]
pub enum Underline {
    /// No underline.
    #[default]
    None,
    /// One straight line (SGR 4).
    Single,
    /// Two straight lines (SGR 21).
    Double,
    /// A wavy line (SGR 4:3).
    Curly,
    /// A dotted line (SGR 4:4).
    Dotted,
    /// A dashed line (SGR 4:5).
    Dashed,
}
