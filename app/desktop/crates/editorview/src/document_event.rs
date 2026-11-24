/// What a document tells the views over it and its owner.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum DocumentEvent {
    /// The text now differs from the saved text, or matches it again.
    DirtyChanged(bool),
    /// Edits paused for the quiet period while the text differs from the saved text: the
    /// moment develop writes a draft and, with Auto Save on, saves.
    EditsPaused,
    /// The document is now the file at another path: its language follows it.
    Rebound,
}
