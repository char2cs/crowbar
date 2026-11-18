//! Fakes shared by the unit tests.
#![cfg(test)]

mod fake_source;

pub(crate) use fake_source::{Events, folder, open, reachable_fake};
