//! Fakes shared by the unit tests: a window that renders one element and reports its bounds.
#![cfg(test)]

mod area;
mod channels;
mod clicks;
mod field;
mod probe;
pub(crate) mod text_layouts;
pub(crate) mod toast_probe;

pub(crate) use area::{area, area_text, area_with};
pub(crate) use channels::channel_byte;
pub(crate) use clicks::press;
pub(crate) use field::{field, text_of};
pub(crate) use probe::{measure, open};
