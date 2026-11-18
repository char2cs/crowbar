//! A fake daemon for unit tests: here, and in crates that test against this client (feature `fake`).

mod fake_daemon;

pub use fake_daemon::{FakeDaemon, Seen};
