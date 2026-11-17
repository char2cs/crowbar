//! Fakes shared by the unit tests: a daemon that answers canned routes and a stream that replays frames.
#![cfg(test)]

mod fake_book;
mod fake_daemon;
mod fake_files;
mod fixture;
mod followed_commands;
mod settings_rig;

pub(crate) use fake_book::{CLIENT, FakeBook, PRINT, desk, fleet_and_desk};
pub(crate) use fake_daemon::{FakeDaemon, local};
pub(crate) use fake_files::{FakeFiles, notes};
pub(crate) use fixture::{frames, quiet, state, state_on, store_of};
pub(crate) use followed_commands::followed_commands;
pub(crate) use settings_rig::{
    AGENTS, AGENTS_PUT, KILL, KILL_PATH, LEVEL_GUARDED, LEVEL_TRUSTED, SETTINGS_PATH, SettingsRig,
    agents_ready,
};
