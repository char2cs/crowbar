/// Whether the user may change the text a view shows.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum Access {
    /// Typing edits the text.
    Editable,
    /// The text can be read, selected and copied, never changed.
    ReadOnly,
}
