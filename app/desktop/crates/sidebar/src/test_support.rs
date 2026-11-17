//! Windows and fakes shared by the unit tests.
#![cfg(test)]

mod blank;
mod memory_preferences;
mod open;
pub(crate) mod prelude;
mod probe;
mod recorder;
mod rows;

pub(crate) use memory_preferences::MemoryPreferences;
pub(crate) use open::open;
pub(crate) use probe::{measure, mount};
pub(crate) use recorder::Recorder;
pub(crate) use rows::{chat, recording_handler};
