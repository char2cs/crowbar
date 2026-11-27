#[derive(Debug, Clone, PartialEq, Eq)]
pub struct PairingRequest {
    pub host_id: String,
    pub endpoint: String,
    pub fingerprint: String,
    pub code: String,
    pub name: String,
}
