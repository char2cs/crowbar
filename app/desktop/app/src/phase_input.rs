use daemon_supervisor::DaemonEvent;
use startup_view::{MigrationStep, Outage, StartupInput};

pub(crate) fn phase_input(event: &DaemonEvent) -> StartupInput {
    match event {
        DaemonEvent::Migrating { progress } => StartupInput::Migrating {
            step: progress
                .as_ref()
                .map(|p| MigrationStep::new(p.step(), p.index(), p.total())),
        },
        DaemonEvent::Ready { .. } => StartupInput::Ready,
        DaemonEvent::Halted { reason, message } => StartupInput::Halted {
            reason: reason.clone(),
            message: message.clone(),
        },
        DaemonEvent::Unhealthy => StartupInput::Unavailable(Outage::Unresponsive),
        DaemonEvent::Recovering {
            attempt, reason, ..
        } => StartupInput::Unavailable(Outage::Recovering {
            attempt: *attempt,
            reason: reason.clone(),
        }),
        DaemonEvent::GaveUp { report } => StartupInput::Unavailable(Outage::GaveUp {
            report: report.clone(),
        }),
        DaemonEvent::Stopped => StartupInput::Unavailable(Outage::Stopped),
        _ => StartupInput::Starting,
    }
}

#[cfg(test)]
mod tests {
    use std::time::Duration;

    use super::*;

    #[test]
    fn every_event_maps_to_its_screen() {
        let cases = [
            (DaemonEvent::Starting, StartupInput::Starting),
            (DaemonEvent::Ready { pid: Some(7) }, StartupInput::Ready),
            (
                DaemonEvent::Migrating { progress: None },
                StartupInput::Migrating { step: None },
            ),
            (
                DaemonEvent::Halted {
                    reason: Some("io_failed".into()),
                    message: "disk".into(),
                },
                StartupInput::Halted {
                    reason: Some("io_failed".into()),
                    message: "disk".into(),
                },
            ),
        ];
        check(cases);
    }

    #[test]
    fn outages_map_to_their_screens() {
        let cases = [
            (
                DaemonEvent::Unhealthy,
                StartupInput::Unavailable(Outage::Unresponsive),
            ),
            (
                DaemonEvent::Recovering {
                    attempt: 2,
                    delay: Duration::from_secs(1),
                    reason: "exit 1".into(),
                },
                StartupInput::Unavailable(Outage::Recovering {
                    attempt: 2,
                    reason: "exit 1".into(),
                }),
            ),
            (
                DaemonEvent::GaveUp { report: "r".into() },
                StartupInput::Unavailable(Outage::GaveUp { report: "r".into() }),
            ),
            (
                DaemonEvent::Stopped,
                StartupInput::Unavailable(Outage::Stopped),
            ),
        ];
        check(cases);
    }

    fn check<const N: usize>(cases: [(DaemonEvent, StartupInput); N]) {
        for (event, expected) in cases {
            assert_eq!(phase_input(&event), expected, "{event:?}");
        }
    }
}
