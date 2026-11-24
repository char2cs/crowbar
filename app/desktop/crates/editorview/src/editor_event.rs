/// What a view tells its owner.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
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
}
