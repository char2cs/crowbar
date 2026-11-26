use startup::{MigrationStep, Outage};
use supervisor::DaemonEvent;

pub(crate) fn phase_input(event: &DaemonEvent) -> startup::Input {
    match event {
        DaemonEvent::Migrating { progress } => startup::Input::Migrating {
            step: progress
                .as_ref()
                .map(|p| MigrationStep::new(p.step(), p.index(), p.total())),
        },
        DaemonEvent::Ready { .. } => startup::Input::Ready,
        DaemonEvent::Restarted => startup::Input::Restarted,
        DaemonEvent::Halted { reason, message } => startup::Input::Halted {
            reason: reason.clone(),
            message: message.clone(),
        },
        DaemonEvent::Unhealthy => startup::Input::Unavailable(Outage::Unresponsive),
        DaemonEvent::Recovering {
            attempt, reason, ..
        } => startup::Input::Unavailable(Outage::Recovering {
            attempt: *attempt,
            reason: reason.clone(),
        }),
        DaemonEvent::GaveUp { report } => startup::Input::Unavailable(Outage::GaveUp {
            report: report.clone(),
        }),
        DaemonEvent::Stopped => startup::Input::Unavailable(Outage::Stopped),
        _ => startup::Input::Starting,
    }
}

#[cfg(test)]
mod tests {
    use std::time::Duration;

    use super::*;

    #[test]
    fn every_event_and_outage_maps_to_its_screen() {
        let cases = [
            (DaemonEvent::Starting, startup::Input::Starting),
            (DaemonEvent::Restarted, startup::Input::Restarted),
            (DaemonEvent::Ready { pid: Some(7) }, startup::Input::Ready),
            (
                DaemonEvent::Migrating { progress: None },
                startup::Input::Migrating { step: None },
            ),
            (
                DaemonEvent::Halted {
                    reason: Some("io_failed".into()),
                    message: "disk".into(),
                },
                startup::Input::Halted {
                    reason: Some("io_failed".into()),
                    message: "disk".into(),
                },
            ),
        ];
        check(cases);
        let outages = [
            (
                DaemonEvent::Unhealthy,
                startup::Input::Unavailable(Outage::Unresponsive),
            ),
            (
                DaemonEvent::Recovering {
                    attempt: 2,
                    delay: Duration::from_secs(1),
                    reason: "exit 1".into(),
                },
                startup::Input::Unavailable(Outage::Recovering {
                    attempt: 2,
                    reason: "exit 1".into(),
                }),
            ),
            (
                DaemonEvent::GaveUp { report: "r".into() },
                startup::Input::Unavailable(Outage::GaveUp { report: "r".into() }),
            ),
            (
                DaemonEvent::Stopped,
                startup::Input::Unavailable(Outage::Stopped),
            ),
        ];
        check(outages);
    }

    fn check<const N: usize>(cases: [(DaemonEvent, startup::Input); N]) {
        for (event, expected) in cases {
            assert_eq!(phase_input(&event), expected, "{event:?}");
        }
    }
}
