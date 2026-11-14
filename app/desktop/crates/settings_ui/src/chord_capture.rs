//! The shortcut currently waiting for a key press.

use crate::capture::Capture;
use crate::chord::Chord;
use crate::pressed::Pressed;
use crate::primary::Primary;
use crate::refusal::Refusal;

#[derive(Debug, Clone, Default, PartialEq, Eq)]
pub(crate) struct ChordCapture {
    target: Option<String>,
    refusal: Option<Refusal>,
}

impl ChordCapture {
    pub(crate) fn begin(&mut self, command: &str) {
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

    pub(crate) fn feed(&mut self, pressed: &Pressed, primary: Primary) -> Option<(String, Chord)> {
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
#[path = "chord_capture_tests.rs"]
mod tests;
