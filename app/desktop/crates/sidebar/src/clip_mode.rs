//! Whether a staged file is copied or moved when pasted.

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub(crate) enum ClipMode {
    Copy,
    Cut,
}
