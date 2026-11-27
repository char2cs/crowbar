//! Feeds daemon frames into the emulator.

use alacritty_terminal::Term;
use alacritty_terminal::event::EventListener;
use alacritty_terminal::vte::ansi::Processor;

use crate::terminal::wire::daemon_frame::DaemonFrame;

const RESET_TO_INITIAL_STATE: &[u8] = b"\x1bc";
const ERASE_SCREEN: &[u8] = b"\x1b[2J";

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
            DaemonFrame::Snapshot(bytes) => self.repaint(term, bytes),
            DaemonFrame::Exit(code) => return Some(*code),
        }
        None
    }

    fn repaint<T: EventListener>(
        &mut self,
        term: &mut Term<T>,
        bytes: &[u8],
    ) {
        self.parser = Processor::default();
        self.parser.advance(term, RESET_TO_INITIAL_STATE);
        let (preamble, content) = bytes.split_at(past_the_erase(bytes));
        self.parser.advance(term, preamble);
        term.grid_mut().clear_history();
        self.parser.advance(term, content);
    }
}

fn past_the_erase(repaint: &[u8]) -> usize {
    repaint
        .windows(ERASE_SCREEN.len())
        .position(|window| window == ERASE_SCREEN)
        .map_or(0, |at| at + ERASE_SCREEN.len())
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::test_support::emulator_showing;
    use alacritty_terminal::grid::Dimensions;

    #[test]
    fn exit_reports_its_code_and_other_frames_report_nothing() {
        let mut feed = Feed::default();
        let mut term = emulator_showing("", 10, 3);
        assert_eq!(
            feed.apply(&mut term, &DaemonFrame::Output(b"x".to_vec())),
            None
        );
        assert_eq!(
            feed.apply(&mut term, &DaemonFrame::Snapshot(b"y".to_vec())),
            None
        );
        assert_eq!(feed.apply(&mut term, &DaemonFrame::Exit(7)), Some(7));
        assert_eq!(term.columns(), 10);
    }

    #[test]
    fn a_repaint_keeps_only_the_scrollback_the_daemon_sent() {
        let cases = [
            ("a fresh prompt", "\x1b[!p\x1b[2J\x1b[H$ ", 0),
            (
                "one line above the screen",
                "\x1b[2J\x1b[Ha\r\nb\r\nc\r\nd",
                1,
            ),
        ];
        for (name, repaint, scrollback) in cases {
            let mut term = emulator_showing("", 10, 3);
            let frame = DaemonFrame::Snapshot(repaint.as_bytes().to_vec());
            Feed::default().apply(&mut term, &frame);
            assert_eq!(term.history_size(), scrollback, "{name}");
        }
    }
}
