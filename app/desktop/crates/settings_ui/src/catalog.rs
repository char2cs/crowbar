//! The pages that ship with the shell.

mod agents;
mod appearance;
mod browser;
mod editor;
mod files;
mod git;
mod hosts;
mod keybindings;
mod rows;
mod sessions;
mod terminal;

use crate::error::Error;
use crate::schema::Schema;

/// Source name of the agents list, to be answered by a [`crate::RecordBackend`].
pub const AGENT_RECORDS: &str = agents::AGENTS;
/// Source name of the descriptor checks list.
pub const DESCRIPTOR_RECORDS: &str = agents::DESCRIPTORS;
/// Source name of the active sessions list.
pub const SESSION_RECORDS: &str = sessions::SESSIONS;
/// Source name of the hosts list.
pub const HOST_RECORDS: &str = hosts::HOSTS;
/// The setting that stores custom keybindings.
pub const KEYBINDING_OVERRIDES: &str = keybindings::OVERRIDES;

/// The ten pages of the shell: Appearance, Editor, Files, Git, Terminal, Browser, Agents, Active
/// sessions, Keybindings and Hosts, in rail order.
///
/// # Errors
/// [`Error::Duplicate`] when the built-in pages repeat an id or key, which the tests rule out.
pub fn default_schema() -> Result<Schema, Error> {
    Schema::new(vec![
        appearance::page(),
        editor::page(),
        files::page(),
        git::page(),
        terminal::page(),
        browser::page(),
        agents::page(),
        sessions::page(),
        keybindings::page(),
        hosts::page(),
    ])
}

#[cfg(test)]
#[path = "catalog_tests.rs"]
mod tests;
