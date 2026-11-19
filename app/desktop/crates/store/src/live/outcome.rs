//! What the shell is told when a command it handed over has finished.

use std::sync::Arc;

use sidebar::{RowId, WorkAtRisk};

/// The result of a command that the shell has to act on.
#[derive(Debug, Clone, PartialEq, Eq)]
pub enum Outcome {
    /// A chat was created and should open in a pane.
    OpenChat(String),
    /// The user asked to install an agent descriptor: the shell picks the YAML file and hands it back.
    PickDescriptor,
    /// The user asked to add a host: the shell opens the pairing dialog.
    PairHost,
    /// The user confirmed clearing browsing data: the shell clears the browser panes' storage.
    ClearBrowsingData,
    /// A file was read for the clipboard: the shell puts this text on it.
    CopyText(String),
    /// The daemon refused or could not be reached; the reason is fit to show.
    Failed(String),
    /// The daemon refused to delete `row` because work on it exists nowhere else: the shell asks
    /// again, naming `at_risk`, before the delete is sent with consent to lose it.
    WorkAtRisk {
        /// The row being removed.
        row: RowId,
        /// The work the delete would lose.
        at_risk: Vec<WorkAtRisk>,
    },
    /// The delete of `row` was refused or failed: the row shows again and `reason`, fit to show,
    /// says why.
    RemovalRefused {
        /// The row being removed.
        row: RowId,
        /// Why it was not removed.
        reason: String,
    },
}

/// Called from a runtime thread, so the shell forwards it onto its own thread.
pub type OutcomeSink = Arc<dyn Fn(Outcome) + Send + Sync>;
