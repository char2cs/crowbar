use daemon_supervisor::DaemonEvent;
use startup_view::{MigrationStep, Outage, StartupInput};

/// What the startup view shows for a supervisor event.
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
#[path = "phase_input_tests.rs"]
mod tests;
