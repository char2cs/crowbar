#[derive(Debug, Clone, PartialEq, Eq)]
pub(crate) enum NameEvent {
    Confirmed(String),
    Dismissed,
}
