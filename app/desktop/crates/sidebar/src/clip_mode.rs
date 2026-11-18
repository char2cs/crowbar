//! Whether a staged file is copied or moved when pasted.

/// What a Paste does with the staged files.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum ClipMode {
    /// Staged by Copy: pasting copies the files and leaves them staged.
    Copy,
    /// Staged by Cut: pasting moves the files.
    Cut,
}
