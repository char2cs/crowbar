#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub(crate) enum DocumentEvent {
    DirtyChanged(bool),
    EditsPaused,
    Rebound,
}
