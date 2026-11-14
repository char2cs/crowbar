use std::path::Path;

use rustix::io::Errno;
use tokio::net::UnixStream;

use crate::health::{Health, check_health, get};

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

pub(crate) fn is_descriptor_exhaustion(error: &std::io::Error) -> bool {
    let raw = error.raw_os_error();
    raw == Some(Errno::MFILE.raw_os_error()) || raw == Some(Errno::NFILE.raw_os_error())
}

/// Deep readiness probe: `/v0/projects` goes through the view store every observed wedge pinned,
/// unlike the static `/v0/health`. Reports whether it could dial at all, since a process out of
/// descriptors cannot reach even a healthy daemon.
pub async fn probe_ready(socket: &Path) -> Probe {
    if let Err(e) = UnixStream::connect(socket).await {
        return if is_descriptor_exhaustion(&e) {
            Probe::LocalDescriptorExhaustion
        } else {
            Probe::Unserved
        };
    }
    match check_health(socket).await {
        Ok(Health::Migrating | Health::Halted { .. }) => return Probe::Held,
        Ok(Health::Ready { .. }) => {},
        Err(_) => return Probe::Unserved,
    }
    match get(socket, "/v0/projects", PROBE_BODY_LIMIT).await {
        Ok(_) => Probe::Healthy,
        Err(_) => Probe::Unserved,
    }
}

#[cfg(test)]
#[path = "probe_tests.rs"]
mod tests;
