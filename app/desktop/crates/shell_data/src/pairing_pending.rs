//! The host's answer the pairing dialog is waiting on.

/// What is in flight: nothing may dismiss the dialog while the host is answering.
#[derive(Debug, Clone, Copy, PartialEq, Eq, Default)]
pub enum PairingPending {
    /// Nothing is out.
    #[default]
    Idle,
    /// The certificate is being read.
    Probing,
    /// The code is being redeemed.
    Pairing,
}
