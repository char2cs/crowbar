#[derive(Debug, Clone, PartialEq, Eq)]
pub(crate) enum PanePhase {
    Parked,
    Loading,
    Mounted,
    Failed(String),
}
