//! What the shell is told when a command it handed over has finished.

use std::collections::BTreeMap;
use std::sync::Arc;

use sidebar::{RowId, WorkAtRisk};

/// The result of a command that the shell has to act on.
#[derive(Debug, Clone, PartialEq, Eq)]
pub enum Outcome {
    /// A chat was created and should open in a pane.
    OpenChat(String),
    /// A new thread opens as a draft pane: no chat exists until its first prompt is sent.
    OpenDraft(crate::live::DraftThread),
    /// The user asked to install an agent descriptor: the shell picks the YAML file and hands it back.
    PickDescriptor,
    /// The user asked to add a host: the shell opens the pairing dialog.
    PairHost,
    /// The user confirmed clearing browsing data: the shell clears the browser panes' storage.
    ClearBrowsingData,
    /// A file was read for the clipboard: the shell puts this text on it.
    CopyText(String),
    /// A row was created to be named at once: the sidebar opens `name` on `row` for editing.
    NameRow {
        /// The new row.
        row: RowId,
        /// The name it was created with.
        name: String,
    },
    /// The daemon answered the name typed into `row`'s inline input.
    NameAnswered {
        /// The row being named.
        row: RowId,
        /// Why the name was refused, fit to show; `None` when it was taken.
        refusal: Option<String>,
    },
    /// The daemon refused or could not be reached; the reason is fit to show.
    Failed(String),
    /// A command failed: shown as a toast titled for the command, such as "Failed to rename
    /// chat", over the daemon's reason.
    Refused {
        /// What failed.
        title: String,
        /// Why, fit to show.
        reason: String,
    },
    /// The daemon refused to delete `row` because work on it exists nowhere else: the shell asks
    /// again, naming `at_risk`, before the delete is sent with consent to lose it.
    WorkAtRisk {
        /// The row being removed.
        row: RowId,
        /// The work the delete would lose.
        at_risk: Vec<WorkAtRisk>,
    },
    /// A workspace's viewed marks changed: the shell sends them to that workspace's review panes.
    ReviewViewed {
        /// The workspace.
        workspace: String,
        /// Path to the signature each file had when it was marked.
        viewed: BTreeMap<String, String>,
    },
    /// A file of the followed workspace was written or created on disk: an editor showing it
    /// reads it again, as develop's `syncBufferWithDisk` does.
    FileChanged {
        /// The workspace the file is in.
        workspace: String,
        /// The file, relative to the workspace.
        path: String,
    },
    /// The followed workspace's file watch was armed again: changes may have been missed, so
    /// every editor showing one of its files reads it again.
    FilesSynced {
        /// The workspace.
        workspace: String,
    },
    /// The delete of `row` was refused or failed: the row shows again and `reason`, fit to show,
    /// says why.
    RemovalRefused {
        /// The row being removed.
        row: RowId,
        /// Why it was not removed.
        reason: String,
    },
    /// The daemon added `folder` as a space or a repository: the form that asked closes.
    Added(String),
    /// A chat delete's git cascade also took chats the user never selected, named by `titles`.
    AlsoRemoved {
        /// The title of each chat taken, in the daemon's order.
        titles: Vec<String>,
    },
}

/// Called from a runtime thread, so the shell forwards it onto its own thread.
pub type OutcomeSink = Arc<dyn Fn(Outcome) + Send + Sync>;
