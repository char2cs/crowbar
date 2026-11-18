//! What a shell data layer is started with besides the hosts it follows.

use crate::live::ChangeSignal;
use crate::live::HostDesk;
use crate::live::OutcomeSink;

/// The paired hosts' book and notes, and the two ways the data layer reaches the shell.
pub struct ShellWiring {
    /// The paired hosts as the transport and the notes file keep them.
    pub desk: HostDesk,
    /// Called after every change of the data.
    pub signal: ChangeSignal,
    /// Called when a command needs the shell to act.
    pub outcome: OutcomeSink,
}

impl std::fmt::Debug for ShellWiring {
    fn fmt(
        &self,
        formatter: &mut std::fmt::Formatter<'_>,
    ) -> std::fmt::Result {
        formatter
            .debug_struct("ShellWiring")
            .finish_non_exhaustive()
    }
}

#[cfg(test)]
mod tests {
    use std::sync::Arc;

    use super::*;
    use crate::test_support::desk;

    #[test]
    fn the_wiring_debugs_without_its_parts() {
        let wiring = ShellWiring {
            desk: desk(),
            signal: Arc::new(|| {}),
            outcome: Arc::new(|_| {}),
        };
        assert_eq!(format!("{wiring:?}"), "ShellWiring { .. }");
    }
}
