#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub(crate) enum ChordLook {
    Idle,
    Capturing,
    Conflict,
}
