//! Windows and fakes shared by the unit tests.
#![cfg(test)]

pub(crate) mod blank;
mod file_choices;
pub(crate) mod open;
pub(crate) mod prelude;
mod probe;
mod recorder;
mod rows;

pub(crate) use file_choices::answer_choices_with;
pub(crate) use open::{Floating, open, open_floating, press, settle};
pub(crate) use probe::{measure, mount};
pub(crate) use recorder::Recorder;
pub(crate) use rows::{chat, recording_handler};
