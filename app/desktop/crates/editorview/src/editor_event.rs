/// What a view tells its owner.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum EditorEvent {
    /// The text now differs from the saved text, or matches it again.
    DirtyChanged(bool),
    /// The user asked to save (ctrl+s).
    SaveRequested,
}
