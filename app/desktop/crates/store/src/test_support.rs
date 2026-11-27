#![cfg(test)]

mod fake_book;
mod fake_files;
mod fixture;
pub(crate) mod followed_commands;
mod local_store;
mod paired_store;
mod settings_rig;

pub(crate) use daemonapi::FakeDaemon;
pub(crate) use fake_book::{CLIENT, FakeBook, PRINT, desk, fleet_and_desk};
pub(crate) use fake_files::{FakeFiles, notes};
pub(crate) use fixture::{frames, local, quiet, state, state_on, store_of};
pub(crate) use followed_commands::followed_commands;
pub(crate) use local_store::local_store;
pub(crate) use paired_store::paired_store;
pub(crate) use settings_rig::{
    AGENTS, AGENTS_PUT, KILL, KILL_PATH, LEVEL_GUARDED, LEVEL_TRUSTED, SETTINGS_PATH, SettingsRig,
    agents_ready,
};
