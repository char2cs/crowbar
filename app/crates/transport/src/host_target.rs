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
mod tests {
    use super::*;

    fn id(text: &str) -> HostId {
        HostId::new(text).unwrap()
    }

    #[test]
    fn local_and_localhost_name_the_local_daemon() {
        assert_eq!(HostTarget::of(&id("local")), HostTarget::Local);
        assert_eq!(HostTarget::of(&id("localhost")), HostTarget::Local);
        assert_eq!(
            HostTarget::of(&id("host-1")),
            HostTarget::Remote(id("host-1"))
        );
    }

    #[test]
    fn the_local_daemon_cannot_be_registered_as_a_remote_host() {
        assert!(check_registrable(&id("host-1")).is_ok());
        for text in ["local", "localhost"] {
            assert!(check_registrable(&id(text)).is_err(), "{text}");
        }
    }
}
