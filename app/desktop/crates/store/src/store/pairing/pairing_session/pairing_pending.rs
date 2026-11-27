#[derive(Debug, Clone, Copy, PartialEq, Eq, Default)]
pub enum PairingPending {
    #[default]
    Idle,
    Probing,
    Pairing,
}
