use gpui::{Entity, Task};

use crate::frame_clock::FrameClock;
use crate::readout_net::ReadoutNet;
use crate::readout_view::ReadoutView;

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

#[cfg(test)]
mod tests {
    use std::time::Instant;

    use gpui::{AppContext as _, TestAppContext};

    use super::*;

    #[gpui::test]
    fn painting_is_clocked_only_while_the_readout_is_shown(cx: &mut TestAppContext) {
        let mut state = ReadoutState::default();
        state.paint();
        assert_eq!(state.clock.stats(Instant::now()).fps, 0, "hidden");
        state.view = Some(cx.new(|_| ReadoutView::default()));
        state.paint();
        assert_eq!(state.clock.stats(Instant::now()).fps, 1, "shown");
    }
}
