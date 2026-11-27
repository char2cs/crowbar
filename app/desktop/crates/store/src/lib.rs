//! Daemon-backed data for the native shell: the sidebar's entities and commands, and the settings
//! and records the settings modal edits.

mod editing;
mod error;
mod journal_sink;
mod live;
mod plan;
mod reads;
mod rows;
mod sidebar_commands;
mod sidebar_view;
mod store;
#[cfg(test)]
mod test_support;

pub use daemonapi::EntryKind;
pub use editing::documents::Documents;
pub use editing::language_servers::LanguageServers;
pub use error::{Error, Result};
pub use live::{
    BookError, BookFuture, ChangeSignal, DraftThread, HostBook, HostDesk, HostNotes, HostSpec,
    Note, Outcome, OutcomeSink, PairingRequest,
};
pub use sidebar_commands::{
    FileDone, FileError, Overwrite, SidebarCommands, TemplateOutcome, ViewedMarks,
};
pub use sidebar_view::SidebarView;
pub use store::{
    Availability, BranchImport, BranchListing, BrowseEntry, DeviceOffer, FolderBrowse,
    FolderChoice, FolderListing, ImportBranch, ImportPlan, LocalLink, Offer, OfferState,
    PairingPending, PairingSession, PairingStage, PairingState, Ports, ProjectsLoad, ROOT_PATH,
    SettingsAdapter, Store, SystemCheck, SystemFonts, Tool, ToolReport,
};
