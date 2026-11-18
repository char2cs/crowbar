//! Daemon-backed data for the native shell: the sidebar's entities and commands, and the settings
//! and records the settings modal edits.

mod error;
mod live;
mod plan;
mod reads;
mod rows;
mod shell_data;
mod sidebar_commands;
mod sidebar_view;
mod target;
#[cfg(test)]
mod test_support;

pub use daemonapi::EntryKind;
pub use error::{Error, Result};
pub use live::{
    BookError, BookFuture, ChangeSignal, ChatPhase, ChatSurface, ChatTerminal, HostBook, HostDesk,
    HostNote, HostNotes, HostSpec, Outcome, OutcomeSink, PairingRequest,
};
pub use shell_data::{
    Availability, BranchImport, BranchListing, BrowseEntry, FolderBrowse, FolderChoice,
    FolderListing, ImportBranch, ImportPlan, LegacyImport, PairingPending, PairingSession,
    PairingStage, PairingState, ROOT_PATH, SettingsAdapter, SettingsCore, ShellData, ShellWiring,
    SystemCheck, SystemFonts, Tool, ToolReport,
};
pub use sidebar_commands::{
    FileDone, FileError, NewEntry, Overwrite, SidebarCommands, TemplateOutcome,
};
pub use sidebar_view::SidebarView;
