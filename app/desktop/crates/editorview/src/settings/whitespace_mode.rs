//! Which spaces and tabs a code editor marks, as Monaco's `renderWhitespace`.

/// Which spaces and tabs a code editor marks.
#[derive(Clone, Copy, Debug, Default, PartialEq, Eq)]
pub enum WhitespaceMode {
    /// No markers.
    #[default]
    None,
    /// Every space and tab except a single space between two words.
    Boundary,
    /// The spaces and tabs after the last other character of a line.
    Trailing,
    /// Every space and tab.
    All,
}
