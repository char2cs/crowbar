//! What the pairing dialog shows.

use crate::shell_data::pairing_session::pairing_pending::PairingPending;
use crate::shell_data::pairing_session::pairing_stage::PairingStage;

/// The stage, the fingerprint to confirm, and why the last step failed, if it did.
#[derive(Debug, Clone, PartialEq, Eq, Default)]
pub struct PairingState {
    /// The current stage.
    pub stage: PairingStage,
    /// What is in flight.
    pub pending: PairingPending,
    /// The fingerprint read from the host; empty before it is known.
    pub fingerprint: String,
    /// Why the last step failed, in plain words.
    pub error: Option<String>,
    /// The correction shown when a code was turned away for its shape.
    pub notice: Option<String>,
}
