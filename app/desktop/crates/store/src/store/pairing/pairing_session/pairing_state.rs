use crate::store::pairing::pairing_session::pairing_pending::PairingPending;
use crate::store::pairing::pairing_session::pairing_stage::PairingStage;

#[derive(Debug, Clone, PartialEq, Eq, Default)]
pub struct PairingState {
    pub stage: PairingStage,
    pub pending: PairingPending,
    pub fingerprint: String,
    pub error: Option<String>,
    pub notice: Option<String>,
}
