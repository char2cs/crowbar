//! What the shell knows of its hosts right now, and what keeps it current: held data, state, the fleet of streams and the panel loaders.

mod file_store;
pub(crate) mod fleet;
mod held;
mod outcome;
pub(crate) mod panel_loader;
mod plan_error;
mod state;

pub(crate) use file_store::{DiskFiles, FileStore, read_text};
pub use fleet::{BookError, BookFuture, HostBook, HostDesk, HostNote, HostNotes, PairingRequest};
pub(crate) use fleet::{Dialled, Fleet, HostRoster, Provider, Surface, relay_for, slot_for};
pub use held::HostSpec;
pub(crate) use held::{
    ChatRecord, EntityStore, FileEntry, FolderRecord, HostSlot, IconCache, LinkState,
    OperationItem, OperationRecord, PanelState, PendingRow, ProjectRecord, RepoRecord,
    SettingsSlot, WorkspaceRecord,
};
pub use outcome::{Outcome, OutcomeSink};
pub(crate) use panel_loader::{DocumentBody, FileTarget, ReloadTarget, load_directory, tree_path};
pub(crate) use plan_error::PlanError;
pub use state::{ChangeSignal, ChatPhase, ChatSurface, ChatTerminal};
pub(crate) use state::{Hub, State, chat_project, workspace_root};
