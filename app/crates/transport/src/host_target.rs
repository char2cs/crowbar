use protocol::HostId;

use crate::error::Error;

const LOCAL_IDS: [&str; 2] = ["local", "localhost"];

#[derive(Debug, Clone, PartialEq, Eq, Hash)]
pub(crate) enum HostTarget {
    Local,
    Remote(HostId),
}

impl HostTarget {
    pub(crate) fn of(host: &HostId) -> Self {
        if LOCAL_IDS.contains(&host.as_str()) {
            return Self::Local;
        }
        Self::Remote(host.clone())
    }
}

pub(crate) fn check_registrable(host: &HostId) -> Result<(), Error> {
    if LOCAL_IDS.contains(&host.as_str()) {
        return Err(Error::BadHostId {
            id: host.clone(),
            why: "it names the local daemon",
        });
    }
    Ok(())
}

#[cfg(test)]
#[path = "host_target_tests.rs"]
mod tests;
