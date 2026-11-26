//! What a shell data layer is started with besides the hosts it follows.

use crate::live::ChangeSignal;
use crate::live::HostDesk;
use crate::live::OutcomeSink;

/// The paired hosts' book and notes, and the two ways the data layer reaches the shell.
pub struct Ports {
    /// The paired hosts as the transport and the notes file keep them.
    pub desk: HostDesk,
    /// Called after every change of the data.
    pub signal: ChangeSignal,
    /// Called when a command needs the shell to act.
    pub outcome: OutcomeSink,
}

impl std::fmt::Debug for Ports {
    fn fmt(
        &self,
        formatter: &mut std::fmt::Formatter<'_>,
    ) -> std::fmt::Result {
        formatter.debug_struct("Ports").finish_non_exhaustive()
    }
}
