//! Fakes shared by the unit tests: a window that renders one element, a recording action sink
//! and a click sweep.
#![cfg(test)]

mod probe;
mod recording_actions;
mod sweep;

pub(crate) use probe::{measure, open};
pub(crate) use recording_actions::RecordingActions;
pub(crate) use sweep::click_across;
