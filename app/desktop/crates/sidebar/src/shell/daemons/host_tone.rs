#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum HostTone {
    Online,
    Offline,
    NeedsAttention,
    Unsettled,
}
