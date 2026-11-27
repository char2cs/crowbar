use crate::daemon::daemon_link::DaemonLink;
use crate::test_support::options_without_a_daemon;

pub(crate) fn link_without_daemon() -> DaemonLink {
    DaemonLink::start(&options_without_a_daemon()).unwrap()
}
