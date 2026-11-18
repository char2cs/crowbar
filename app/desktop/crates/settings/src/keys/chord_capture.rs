//! The shortcut currently waiting for a key press.

use crate::keys::capture::Capture;
use crate::keys::pressed::Pressed;
use crate::keys::refusal::Refusal;
use chord::Chord;
use chord::Primary;

#[derive(Debug, Clone, Default, PartialEq, Eq)]
pub(crate) struct ChordCapture {
    target: Option<String>,
    refusal: Option<Refusal>,
}

impl ChordCapture {
    pub(crate) fn begin(
        &mut self,
        command: &str,
    ) {
        if self.target.as_deref() == Some(command) {
            self.cancel();
            return;
        }
        self.target = Some(command.to_owned());
        self.refusal = None;
    }

    pub(crate) fn cancel(&mut self) {
        self.target = None;
        self.refusal = None;
    }

    pub(crate) fn target(&self) -> Option<&str> {
        self.target.as_deref()
    }

    pub(crate) fn refusal(&self) -> Option<Refusal> {
        self.refusal
    }

    pub(crate) fn feed(
        &mut self,
        pressed: &Pressed,
        primary: Primary,
    ) -> Option<(String, Chord)> {
        let command = self.target.clone()?;
        match Capture::interpret(pressed, primary) {
            Capture::Ignored => None,
            Capture::Cancelled => {
                self.cancel();
                None
            },
            Capture::Refused(refusal) => {
                self.refusal = Some(refusal);
                None
            },
            Capture::Accepted(chord) => {
                self.cancel();
                Some((command, chord))
            },
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::keys::modifier::Modifier;
    use crate::test_support::cases::{Case, run_cases};

    fn press(
        key: &str,
        held: &[Modifier],
    ) -> Pressed {
        Pressed::new(key, held)
    }

    fn idle_capture_ignores_everything() {
        let mut capture = ChordCapture::default();
        assert_eq!(
            capture.feed(&press("t", &[Modifier::Control]), Primary::Control),
            None
        );
        assert_eq!(capture.target(), None);
    }

    fn an_accepted_chord_ends_capture_and_names_the_command() {
        let mut capture = ChordCapture::default();
        capture.begin("tabs.new");
        let done = capture.feed(&press("n", &[Modifier::Control]), Primary::Control);
        assert_eq!(
            done.map(|(command, chord)| (command, chord.as_str().to_owned())),
            Some(("tabs.new".to_owned(), "mod+n".to_owned()))
        );
        assert_eq!(capture.target(), None);
    }

    fn refusals_keep_capturing_and_remember_why() {
        let mut capture = ChordCapture::default();
        capture.begin("tabs.new");
        assert_eq!(capture.feed(&press("a", &[]), Primary::Control), None);
        assert_eq!(capture.target(), Some("tabs.new"));
        assert_eq!(capture.refusal(), Some(Refusal::BareKey));
        assert_eq!(
            capture.feed(&press("control", &[Modifier::Control]), Primary::Control),
            None
        );
        assert_eq!(
            capture.target(),
            Some("tabs.new"),
            "a bare modifier keeps waiting"
        );
    }

    fn escape_and_a_second_begin_both_leave_capture() {
        let mut capture = ChordCapture::default();
        capture.begin("tabs.new");
        assert_eq!(capture.feed(&press("escape", &[]), Primary::Control), None);
        assert_eq!(capture.target(), None);
        capture.begin("tabs.new");
        capture.begin("tabs.new");
        assert_eq!(
            capture.target(),
            None,
            "pressing the capturing button again toggles it off"
        );
        capture.begin("tabs.new");
        capture.begin("tabs.close");
        assert_eq!(
            capture.target(),
            Some("tabs.close"),
            "another button takes over"
        );
    }

    const CASES: &[Case] = &[
        (
            "idle capture ignores everything",
            idle_capture_ignores_everything,
        ),
        (
            "an accepted chord ends capture and names the command",
            an_accepted_chord_ends_capture_and_names_the_command,
        ),
        (
            "refusals keep capturing and remember why",
            refusals_keep_capturing_and_remember_why,
        ),
        (
            "escape and a second begin both leave capture",
            escape_and_a_second_begin_both_leave_capture,
        ),
    ];

    #[test]
    fn every_scenario_holds() {
        run_cases(CASES);
    }
}
