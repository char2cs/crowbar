//! Feeds daemon frames into the emulator.

use alacritty_terminal::Term;
use alacritty_terminal::event::EventListener;
use alacritty_terminal::vte::ansi::Processor;

use crate::terminal::wire::daemon_frame::DaemonFrame;

const RESET_TO_INITIAL_STATE: &[u8] = b"\x1bc";

#[derive(Default)]
pub(crate) struct Feed {
    parser: Processor,
}

impl Feed {
    pub(crate) fn apply<T: EventListener>(
        &mut self,
        term: &mut Term<T>,
        frame: &DaemonFrame,
    ) -> Option<i32> {
        match frame {
            DaemonFrame::Output(bytes) => self.parser.advance(term, bytes),
            DaemonFrame::Snapshot(bytes) => {
                self.parser = Processor::default();
                self.parser.advance(term, RESET_TO_INITIAL_STATE);
                self.parser.advance(term, bytes);
            },
            DaemonFrame::Exit(code) => return Some(*code),
        }
        None
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::test_support::emulator_showing;

    #[test]
    fn exit_reports_its_code_and_other_frames_report_nothing() {
        let mut feed = Feed::default();
        let mut term = emulator_showing("", 10, 3);
        let frames = [
            DaemonFrame::Output(b"x".to_vec()),
            DaemonFrame::Snapshot(b"y".to_vec()),
            DaemonFrame::Exit(7),
        ];
        let codes: Vec<Option<i32>> = frames
            .iter()
            .map(|frame| feed.apply(&mut term, frame))
            .collect();
        assert_eq!(codes, [None, None, Some(7)]);
    }
}
