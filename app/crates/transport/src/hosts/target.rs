use protocol::HostId;

use crate::error::Error;

const LOCAL_IDS: [&str; 2] = ["local", "localhost"];

#[derive(Debug, Clone, PartialEq, Eq, Hash)]
pub(crate) enum Target {
    Local,
    Remote(HostId),
}

impl Target {
    pub(crate) fn of(host: &HostId) -> Self {
        if LOCAL_IDS.contains(&host.as_str()) {
            return Self::Local;
        }
        Self::Remote(host.clone())
    }
}

pub(crate) fn check_registrable(host: &HostId) -> Result<(), Error> {
    let names_local_daemon = LOCAL_IDS.contains(&host.as_str());
    (!names_local_daemon)
        .then_some(())
        .ok_or_else(|| Error::BadHostId {
            id: host.clone(),
            why: "it names the local daemon",
        })
}

#[cfg(test)]
mod tests {
    use super::*;

    fn id(text: &str) -> HostId {
        HostId::new(text).unwrap()
    }

    #[derive(Clone, Copy)]
    enum Names {
        LocalDaemon,
        Host,
    }

    const CASES: [(&str, Names); 3] = [
        ("local", Names::LocalDaemon),
        ("localhost", Names::LocalDaemon),
        ("host-1", Names::Host),
    ];

    #[test]
    fn local_and_localhost_name_the_local_daemon() {
        for (text, names) in CASES {
            let expected = match names {
                Names::LocalDaemon => Target::Local,
                Names::Host => Target::Remote(id(text)),
            };
            assert_eq!(Target::of(&id(text)), expected, "{text}");
        }
    }

    #[test]
    fn the_local_daemon_cannot_be_registered_as_a_remote_host() {
        for (text, names) in CASES {
            let refused = matches!(names, Names::LocalDaemon);
            assert_eq!(check_registrable(&id(text)).is_err(), refused, "{text}");
        }
    }
}
