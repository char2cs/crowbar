//! Fakes shared by the unit tests: a window that renders one element and reports its bounds.
#![cfg(test)]

pub(crate) mod channels;
mod contrast;
pub(crate) mod floats;
pub(crate) mod menus;
pub(crate) mod probe;

pub(crate) use contrast::contrast_ratio;
