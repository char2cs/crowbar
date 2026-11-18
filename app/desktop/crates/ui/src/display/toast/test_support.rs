//! A toast queue in a test window, shared by the toast and timer tests.

mod toast_probe;

pub(crate) use toast_probe::{advance, push, queue};
