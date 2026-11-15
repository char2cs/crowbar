//! Fakes shared by the unit tests: a daemon that answers canned routes and a stream that replays frames.
#![cfg(test)]

mod fake_daemon;
mod fixture;

pub(crate) use fake_daemon::{FakeDaemon, local};
pub(crate) use fixture::{frames, state, store_of};
