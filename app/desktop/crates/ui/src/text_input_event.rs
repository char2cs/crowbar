//! Events a text input emits.

/// What happened in a [`crate::TextInput`].
#[derive(Debug, Clone, PartialEq, Eq)]
#[non_exhaustive]
pub enum TextInputEvent {
    /// The text changed; carries the new text.
    Changed(String),
    /// The user pressed `Enter`.
    Submitted,
    /// The user pressed `Escape`.
    Cancelled,
}
