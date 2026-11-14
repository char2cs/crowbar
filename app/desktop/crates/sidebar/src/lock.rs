//! Whether a branch is locked against renames and removal.

/// Locked branches show a lock glyph and refuse rename and remove.
#[derive(Debug, Clone, Copy, PartialEq, Eq, Default)]
pub enum Lock {
    /// Free to change.
    #[default]
    Unlocked,
    /// Protected.
    Locked,
}
