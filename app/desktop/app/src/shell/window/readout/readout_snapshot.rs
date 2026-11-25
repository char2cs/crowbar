use crate::shell::window::readout::frame_stats::FrameStats;
use crate::shell::window::readout::readout_net::ReadoutNet;

#[derive(Debug, Clone, PartialEq)]
pub(crate) struct ReadoutSnapshot {
    pub(crate) build: String,
    pub(crate) net: ReadoutNet,
    pub(crate) reconnects: u32,
    pub(crate) frame: FrameStats,
    pub(crate) counts: (usize, usize, usize),
}
