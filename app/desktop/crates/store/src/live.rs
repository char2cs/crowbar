//! What the shell knows of its hosts right now, and what keeps it current: held data, state, the fleet of streams and the panel loaders.

mod files;
pub(crate) mod fleet;
mod held;
mod outcome;
pub(crate) mod panel_loader;
mod plan_error;
mod state;

pub(crate) use files::{DiskFiles, Files, read_text};
pub use fleet::{BookError, BookFuture, HostBook, HostDesk, HostNotes, Note, PairingRequest};
pub(crate) use fleet::{
    Dialled, Fleet, Provider, Roster, StreamControl, Surface, relay_for, slot_for,
};
pub use held::HostSpec;
pub(crate) use held::{
    ChatRecord, Entities, FileEntry, FolderRecord, HostSlot, IconCache, Item, LinkState,
    OperationRecord, PanelState, PendingRow, PlacementAnswer, ProjectRecord, RepoRecord,
    SettingsSlot, ViewedFiles, WorkspaceRecord,
};
pub use outcome::{Outcome, OutcomeSink};
pub(crate) use panel_loader::{DocumentBody, FileTarget, ReloadTarget, load_directory, tree_path};
pub(crate) use plan_error::PlanError;
pub use state::{ChangeSignal, ChatPhase, ChatSurface, ChatTerminal};
pub(crate) use state::{Hub, State, chat_project, workspace_root};
