#[derive(Debug, Clone, PartialEq, Eq)]
pub(crate) enum RenameEvent {
    Confirmed(String),
    Dismissed,
}
