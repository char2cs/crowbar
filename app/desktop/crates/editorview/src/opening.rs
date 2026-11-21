/// What a view opens on: the document's text, its path, and where to put the caret.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct Opening {
    /// The document's text.
    pub text: String,
    /// The path the toolbar's breadcrumb shows and the language comes from; `None` for an
    /// untitled document, which shows no toolbar.
    pub path: Option<String>,
    /// The 1-based line to reveal, as a terminal's `path:line` link asks; `None` for the top.
    pub line: Option<u32>,
    /// Whether typing edits it.
    pub access: crate::Access,
}
