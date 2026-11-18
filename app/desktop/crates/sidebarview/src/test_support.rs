//! Windows and fakes shared by the unit tests.
#![cfg(test)]

mod blank;
pub(crate) mod open;
pub(crate) mod prelude;
mod probe;
mod recorder;
mod rows;

pub(crate) use open::open;
pub(crate) use probe::{measure, mount};
pub(crate) use recorder::Recorder;
pub(crate) use rows::{chat, recording_handler};
