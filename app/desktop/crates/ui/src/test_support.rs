//! Fakes shared by the unit tests: a window that renders one element and reports its bounds.

mod probe;

pub(crate) use probe::{Probe, measure, open};
