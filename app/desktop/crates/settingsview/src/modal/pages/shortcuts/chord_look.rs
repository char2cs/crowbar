//! The state a chord button shows.

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub(crate) enum ChordLook {
    Idle,
    Capturing,
    Conflict,
}
