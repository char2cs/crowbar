use std::sync::Arc;

use gpui::{Entity, Task};

use crate::shell::ports::clock::Clock;
use crate::shell::window::readout::frame_clock::FrameClock;
use crate::shell::window::readout::readout_net::ReadoutNet;
use crate::shell::window::readout::readout_view::ReadoutView;

pub(crate) struct ReadoutState {
    pub(crate) view: Option<Entity<ReadoutView>>,
    pub(crate) tick: Option<Task<()>>,
    pub(crate) clock: FrameClock,
    pub(crate) reconnects: u32,
    pub(crate) net: ReadoutNet,
    pub(crate) time: Arc<dyn Clock>,
}

impl ReadoutState {
    pub(crate) fn new(time: Arc<dyn Clock>) -> Self {
        Self {
            view: None,
            tick: None,
            clock: FrameClock::default(),
            reconnects: 0,
            net: ReadoutNet::default(),
            time,
        }
    }

    pub(crate) fn paint(&mut self) {
        if self.view.is_some() {
            self.clock.record(self.time.instant());
        }
    }
}

#[cfg(test)]
mod tests {
    use gpui::{AppContext as _, TestAppContext};

    use super::*;
    use crate::test_support::FixedClock;

    #[gpui::test]
    fn painting_is_clocked_only_while_the_readout_is_shown(cx: &mut TestAppContext) {
        let time = Arc::new(FixedClock::at(0));
        let mut state = ReadoutState::new(Arc::clone(&time) as Arc<dyn Clock>);
        state.paint();
        assert_eq!(state.clock.stats(time.instant()).fps, 0, "hidden");
        state.view = Some(cx.new(|_| ReadoutView::default()));
        state.paint();
        assert_eq!(state.clock.stats(time.instant()).fps, 1, "shown");
    }
}
