//! The data the state holds, with no async and no hub: host slots and specs, links, pending rows, icons, settings, entities and panel state.

mod entities;
mod host_slot;
mod host_spec;
mod icon_cache;
mod link_state;
mod open_threads;
mod panel_state;
mod pending_row;
mod settings_slot;
mod viewed_files;

pub(crate) use entities::{
    ChatRecord, Entities, FolderRecord, Item, OperationRecord, PROJECTS_SCOPE, ProjectRecord,
    RepoRecord, WorkspaceRecord,
};
pub(crate) use host_slot::HostSlot;
pub use host_spec::HostSpec;
pub(crate) use icon_cache::IconCache;
pub(crate) use link_state::LinkState;
pub(crate) use open_threads::OpenThreads;
pub(crate) use panel_state::{FileEntry, PanelState};
pub(crate) use pending_row::PendingRow;
pub(crate) use settings_slot::SettingsSlot;
pub(crate) use viewed_files::ViewedFiles;
