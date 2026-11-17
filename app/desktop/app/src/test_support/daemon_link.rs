use super::options_without_a_daemon;
use crate::daemon_link::DaemonLink;

pub(crate) fn link_without_daemon() -> DaemonLink {
    DaemonLink::start(&options_without_a_daemon()).unwrap()
}
