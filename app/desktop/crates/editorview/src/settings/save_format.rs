//! Whether a manual save formats first, as develop's `formatOnSave`.

/// What a manual save does to the document before writing it.
#[derive(Debug, Clone, Copy, Default, PartialEq, Eq)]
pub enum SaveFormat {
    /// The text is written as it stands.
    #[default]
    AsTyped,
    /// The language server formats the document first.
    Formatted,
}
