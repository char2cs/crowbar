//! Where the pairing dialog is.

/// The stages of adding a host, in order.
#[derive(Debug, Clone, Copy, PartialEq, Eq, Default)]
pub enum PairingStage {
    /// Asking for the host's name and address.
    #[default]
    Address,
    /// Showing the certificate fingerprint to compare with the host's own.
    Fingerprint,
    /// Asking for the one-time code.
    Code,
    /// The host is paired and followed.
    Paired,
}
