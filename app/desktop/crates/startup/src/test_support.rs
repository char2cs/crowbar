//! Fakes shared by the unit tests: a window that renders one element and a recording action sink.
#![cfg(test)]

mod probe;
mod recording_actions;

pub(crate) use probe::measure;
pub(crate) use recording_actions::RecordingActions;
