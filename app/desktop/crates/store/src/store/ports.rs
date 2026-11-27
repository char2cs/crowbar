use crate::live::ChangeSignal;
use crate::live::HostDesk;
use crate::live::OutcomeSink;

pub struct Ports {
    pub desk: HostDesk,
    pub signal: ChangeSignal,
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

#[cfg(test)]
mod tests {
    use std::sync::Arc;

    use super::*;
    use crate::test_support::desk;

    #[test]
    fn the_wiring_debugs_without_its_parts() {
        let wiring = Ports {
            desk: desk(),
            signal: Arc::new(|| {}),
            outcome: Arc::new(|_| {}),
        };
        assert_eq!(format!("{wiring:?}"), "Ports { .. }");
    }
}
