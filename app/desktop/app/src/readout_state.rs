use gpui::{Entity, Task};

use crate::frame_clock::FrameClock;
use crate::readout_net::ReadoutNet;
use crate::readout_view::ReadoutView;

/// The readout while the `buildReadout` setting shows it.
#[derive(Default)]
pub(crate) struct ReadoutState {
    pub(crate) view: Option<Entity<ReadoutView>>,
    pub(crate) tick: Option<Task<()>>,
    pub(crate) clock: FrameClock,
    pub(crate) reconnects: u32,
    pub(crate) net: ReadoutNet,
}

impl ReadoutState {
    pub(crate) fn paint(&mut self) {
        if self.view.is_some() {
            self.clock.record(std::time::Instant::now());
        }
    }
}
