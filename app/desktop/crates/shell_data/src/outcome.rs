//! What the shell is told when a command it handed over has finished.

use std::sync::Arc;

/// The result of a command that the shell has to act on.
#[derive(Debug, Clone, PartialEq, Eq)]
pub enum Outcome {
    /// A chat was created and should open in a pane.
    OpenChat(String),
    /// The user asked to install an agent descriptor: the shell picks the YAML file and hands it back.
    PickDescriptor,
    /// The user asked to add a host: the shell opens the pairing dialog.
    PairHost,
    /// A file was read for the clipboard: the shell puts this text on it.
    CopyText(String),
    /// The daemon refused or could not be reached; the reason is fit to show.
    Failed(String),
}

/// Called from a runtime thread, so the shell forwards it onto its own thread.
pub type OutcomeSink = Arc<dyn Fn(Outcome) + Send + Sync>;
