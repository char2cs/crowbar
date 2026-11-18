/// What the dialog asks of the shell.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum PairingEvent {
    /// The user dismissed the dialog, or is done with it.
    Close,
    /// The host is paired and followed.
    Paired,
}
