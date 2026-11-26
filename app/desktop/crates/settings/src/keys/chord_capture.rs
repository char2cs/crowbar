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

    #[test]
    fn a_second_begin_toggles_off_and_another_command_takes_over() {
        let mut capture = ChordCapture::default();
        capture.begin("tabs.new");
        capture.begin("tabs.new");
        assert_eq!(capture.target(), None, "the capturing button toggles off");
        capture.begin("tabs.new");
        capture.begin("tabs.close");
        assert_eq!(
            capture.target(),
            Some("tabs.close"),
            "another button takes over"
        );
    }
}
