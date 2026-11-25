/// Where the caret is, 1-based, as the toolbar's chip shows it.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub struct Caret {
    /// The line.
    pub line: u32,
    /// The column.
    pub column: u32,
}
