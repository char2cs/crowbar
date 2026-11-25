#[derive(Debug, Clone, Copy, PartialEq, Eq, Default)]
pub(crate) struct FrameStats {
    pub(crate) fps: usize,
    pub(crate) max_gap_ms: u64,
    pub(crate) drops: usize,
}
