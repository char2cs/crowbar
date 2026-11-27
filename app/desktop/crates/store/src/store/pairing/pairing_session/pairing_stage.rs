#[derive(Debug, Clone, Copy, PartialEq, Eq, Default)]
pub enum PairingStage {
    #[default]
    Address,
    Fingerprint,
    Code,
    Paired,
}
