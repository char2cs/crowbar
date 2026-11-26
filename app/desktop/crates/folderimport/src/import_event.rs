//! What the dialog asks of its owner.

/// What the dialog asks of its owner.
#[derive(Debug, Clone, PartialEq, Eq)]
pub enum ImportEvent {
    /// Choose a folder; the answer goes to [`crate::View::set_folder`].
    Browse,
    /// Add `folder` under `name`; a blank name means the folder's own.
    Add {
        /// The absolute folder path.
        folder: String,
        /// The typed name, trimmed.
        name: String,
    },
    /// The dialog was cancelled or the folder was added.
    Close,
}
