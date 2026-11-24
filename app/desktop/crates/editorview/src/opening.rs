/// Where a view over a document puts its caret, and whether typing edits it.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub struct Opening {
    /// The 1-based line to reveal, as a terminal's `path:line` link asks; `None` for the top.
    pub line: Option<u32>,
    /// Whether typing edits it.
    pub access: crate::Access,
}
