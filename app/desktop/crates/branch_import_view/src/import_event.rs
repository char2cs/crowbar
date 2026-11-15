/// What the dialog asks of its host.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum ImportEvent {
    /// The dialog was dismissed or the import was started: take it down.
    Close,
}
