//! In-memory fakes shared by the unit tests of this crate.

pub(crate) mod fake_daemon;

pub(crate) mod memory_sink;

pub(crate) mod supervision;

pub(crate) use supervision::supervision;
