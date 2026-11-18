//! How the cursor keys are encoded.

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub(crate) enum CursorKeys {
    Normal,
    Application,
}
