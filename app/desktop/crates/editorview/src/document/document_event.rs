#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum DocumentEvent {
    DirtyChanged(bool),
    EditsPaused,
    Rebound,
}
