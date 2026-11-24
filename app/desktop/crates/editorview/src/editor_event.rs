/// What a view tells its owner.
#[derive(Debug, Clone, PartialEq, Eq)]
pub enum EditorEvent {
    /// The text now differs from the saved text, or matches it again.
    DirtyChanged(bool),
    /// The user asked to save (ctrl+s).
    SaveRequested,
    /// Edits paused for the quiet period while the text differs from the saved text: the
    /// moment develop writes a draft and, with Auto Save on, saves.
    EditsPaused,
    /// The user asked to preview the document (the toolbar's Preview button).
    PreviewRequested,
    /// The user asked to show the document in its rich view instead (Markdown only).
    RichViewRequested,
    /// Go to definition led to another workspace file, at an LSP position.
    Definition {
        /// The file's workspace-relative path.
        path: String,
        /// The definition's 0-based line.
        line: u32,
        /// The definition's column, in UTF-16 code units as LSP counts them.
        character: u32,
    },
    /// Go to definition led only outside the workspace, to this absolute path.
    DefinitionOutside(String),
}
