use crate::error::Error;
use crate::slot::HostEvent;

/// What one reconcile produced. Events are never lost to an error: a pane that failed to be
/// driven is reported in `failure` while the others were still reconciled.
#[derive(Debug, Default)]
pub struct SyncReport {
    /// What the shell must apply to the layout.
    pub events: Vec<HostEvent>,
    /// The first error met, when any pane could not be driven.
    pub failure: Option<Error>,
}
