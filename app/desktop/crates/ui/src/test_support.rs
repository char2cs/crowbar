//! Fakes shared by the unit tests: a window that renders one element and reports its bounds.
#![cfg(test)]

mod channels;
mod field;
mod probe;

pub(crate) use channels::channel_byte;
pub(crate) use field::{field, text_of};
pub(crate) use probe::{measure, open};
