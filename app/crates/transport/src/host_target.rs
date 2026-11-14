use protocol::HostId;

use crate::error::Error;

const LOCAL_IDS: [&str; 2] = ["local", "localhost"];

#[derive(Debug, Clone, PartialEq, Eq, Hash)]
/// Where a host id points: this machine's own daemon, or a paired remote host.
pub enum HostTarget {
    /// The daemon on this machine, reached over its unix socket.
    Local,
    /// A paired remote host.
    Remote(HostId),
}

impl HostTarget {
    /// Classifies `host`: `local` and `localhost` are this machine, anything else is remote.
    #[must_use]
    pub fn of(host: &HostId) -> Self {
        if LOCAL_IDS.contains(&host.as_str()) {
            return Self::Local;
        }
        Self::Remote(host.clone())
    }
}

pub(crate) fn check_registrable(host: &HostId) -> Result<(), Error> {
    let bad = |why| Error::BadHostId {
        id: host.clone(),
        why,
    };
    if LOCAL_IDS.contains(&host.as_str()) {
        return Err(bad("it names the local daemon"));
    }
    if host.as_str().contains('_') {
        return Err(bad(
            "remote host ids hold only lowercase letters, digits and hyphens",
        ));
    }
    Ok(())
}

#[cfg(test)]
#[path = "host_target_tests.rs"]
mod tests;
