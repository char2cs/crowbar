use crate::frame_stats::FrameStats;
use crate::readout_net::ReadoutNet;

/// Everything the readout shows, as read at one moment. `counts` are the open terminals, the
/// running agents and the open spaces.
#[derive(Debug, Clone, PartialEq)]
pub(crate) struct ReadoutSnapshot {
    pub(crate) build: String,
    pub(crate) net: ReadoutNet,
    pub(crate) reconnects: u32,
    pub(crate) frame: FrameStats,
    pub(crate) counts: (usize, usize, usize),
}
