use std::path::Path;

use crate::daemon_get::get;
use crate::dial::Dial;
use crate::error::Error;
use crate::health::{Health, check_health};
use crate::socket_dial::SocketDial;

const PROBE_BODY_LIMIT: usize = 8 * 1024 * 1024;

/// What a readiness probe learned. A failure has two causes the watchdog must never confuse:
/// the daemon is not serving, or this process has no descriptors left to dial with.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum Probe {
    /// The daemon answered.
    Healthy,
    /// The daemon was dialled and did not serve the request.
    Unserved,
    /// This process hit its own open-file limit while dialling; says nothing about the daemon.
    LocalDescriptorExhaustion,
    /// The daemon is alive but deliberately not serving: preparing its home, or halted.
    Held,
}

fn failed(error: &Error) -> Probe {
    if error.is_descriptor_exhaustion() {
        Probe::LocalDescriptorExhaustion
    } else {
        Probe::Unserved
    }
}

pub(crate) async fn probe_with<D: Dial>(dial: &D) -> Probe {
    match check_health(dial).await {
        Err(error) => return failed(&error),
        Ok(Health::Migrating { .. } | Health::Halted { .. }) => return Probe::Held,
        Ok(Health::Ready { .. }) => {},
    }
    match get(dial, "/v0/projects", PROBE_BODY_LIMIT).await {
        Ok(_) => Probe::Healthy,
        Err(error) => failed(&error),
    }
}

/// Deep readiness probe: `/v0/projects` goes through the view store every observed wedge pinned,
/// unlike the static `/v0/health`. Every dial is classified, not just the first: a process out of
/// descriptors cannot reach even a healthy daemon, whichever request ran out.
pub async fn probe_ready(socket: &Path) -> Probe {
    probe_with(&SocketDial::new(socket)).await
}

#[cfg(test)]
#[path = "probe_tests.rs"]
mod tests;
