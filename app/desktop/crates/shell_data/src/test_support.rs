//! Fakes shared by the unit tests: a daemon that answers canned routes and a stream that replays frames.
#![cfg(test)]

mod fake_book;
mod fake_daemon;
mod fixture;

pub(crate) use fake_book::{CLIENT, FakeBook, PRINT, desk, fleet_and_desk};
pub(crate) use fake_daemon::{FakeDaemon, local};
pub(crate) use fixture::{frames, state, state_on, store_of};
