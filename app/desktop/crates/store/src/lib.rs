//! Daemon-backed data for the native shell: the sidebar's entities and commands, and the settings
//! and records the settings modal edits.

mod error;
mod live;
mod plan;
mod reads;
mod rows;
mod sidebar_commands;
mod sidebar_view;
mod store;
mod target;
#[cfg(test)]
mod test_support;

pub use daemonapi::EntryKind;
pub use error::{Error, Result};
pub use live::{
    BookError, BookFuture, ChangeSignal, ChatPhase, ChatSurface, ChatTerminal, HostBook, HostDesk,
    HostNotes, HostSpec, Note, Outcome, OutcomeSink, PairingRequest,
};
pub use sidebar_commands::{
    FileDone, FileError, Overwrite, SidebarCommands, TemplateOutcome, ViewedMarks,
};
pub use sidebar_view::SidebarView;
pub use store::{
    Availability, BranchImport, BranchListing, BrowseEntry, FolderBrowse, FolderChoice,
    FolderListing, ImportBranch, ImportPlan, LegacyImport, LocalLink, PairingPending,
    PairingSession, PairingStage, PairingState, Ports, ProjectsLoad, ROOT_PATH, SettingsAdapter,
    Store, SystemCheck, SystemFonts, Tool, ToolReport,
};
