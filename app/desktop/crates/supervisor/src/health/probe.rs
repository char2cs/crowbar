use std::path::Path;

use crate::error::Error;
use crate::health::client::Dial;
use crate::health::client::SocketDial;
use crate::health::client::get;
use crate::health::{Health, check_health};

const PROBE_BODY_LIMIT: usize = 8 * 1024 * 1024;

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub(crate) enum Probe {
    Healthy,
    Unserved,
    LocalDescriptorExhaustion,
    Held,
}

fn failed(error: &Error) -> Probe {
    if error.is_descriptor_exhaustion() {
        return Probe::LocalDescriptorExhaustion;
    }
    Probe::Unserved
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

pub(crate) async fn probe_ready(socket: &Path) -> Probe {
    probe_with(&SocketDial::new(socket)).await
}

#[cfg(test)]
mod tests {
    use std::io::ErrorKind;

    use rustix::io::Errno;

    use super::*;
    use crate::supervisor::monitoring::watchdog::failure_tracker::FailureTracker;
    use crate::test_support::fake_daemon::FakeDaemon;

    const OK: &str =
        include_str!("../../../../../../api/internal/core/bootgate/testdata/health-ok.json");
    const MIGRATING: &str =
        include_str!("../../../../../../api/internal/core/bootgate/testdata/health-migrating.json");
    const HALTED: &str =
        include_str!("../../../../../../api/internal/core/bootgate/testdata/health-halted.json");
    const THRESHOLD: u32 = 6;

    fn daemon(
        health: &'static str,
        other: u16,
    ) -> FakeDaemon {
        FakeDaemon::answering(move |_, target| match target {
            "/v0/health" => (200, health.to_owned()),
            _ => (other, "{}".to_owned()),
        })
    }

    fn os(errno: Errno) -> std::io::Error {
        std::io::Error::from_raw_os_error(errno.raw_os_error())
    }

    fn failing_dial(
        nth: usize,
        errno: Errno,
    ) -> FakeDaemon {
        daemon(OK, 200).refusing(move |n| (n == nth).then(|| os(errno)))
    }

    #[tokio::test]
    async fn each_daemon_answer_maps_to_its_probe_outcome() {
        let missing = daemon(OK, 200).refusing(|_| Some(std::io::Error::from(ErrorKind::NotFound)));
        let exhausted = Probe::LocalDescriptorExhaustion;
        let cases = [
            ("migrating", daemon(MIGRATING, 503), Probe::Held),
            ("halted", daemon(HALTED, 503), Probe::Held),
            ("deep probe fails", daemon(OK, 503), Probe::Unserved),
            ("serving", daemon(OK, 200), Probe::Healthy),
            (
                "health dial EMFILE",
                failing_dial(0, Errno::MFILE),
                exhausted,
            ),
            (
                "health dial ENFILE",
                failing_dial(0, Errno::NFILE),
                exhausted,
            ),
            (
                "projects dial EMFILE",
                failing_dial(1, Errno::MFILE),
                exhausted,
            ),
            (
                "projects dial ENFILE",
                failing_dial(1, Errno::NFILE),
                exhausted,
            ),
            (
                "refused",
                failing_dial(0, Errno::CONNREFUSED),
                Probe::Unserved,
            ),
            ("missing socket", missing, Probe::Unserved),
        ];
        assert_outcomes(cases).await;
    }

    async fn assert_outcomes(cases: [(&str, FakeDaemon, Probe); 10]) {
        for (name, daemon, expected) in cases {
            assert_eq!(probe_with(&daemon).await, expected, "{name}");
        }
    }

    #[tokio::test]
    async fn a_healthy_daemon_is_never_indicted_however_often_a_dial_runs_dry() {
        let daemon = daemon(OK, 200).refusing(|n| (n % 2 == 1).then(|| os(Errno::MFILE)));
        assert_never_indicted(&daemon).await;
    }

    async fn assert_never_indicted(daemon: &FakeDaemon) {
        let mut tracker = FailureTracker::new(THRESHOLD);
        for _ in 0..THRESHOLD * 2 {
            let outcome = probe_with(daemon).await;
            assert_eq!(outcome, Probe::LocalDescriptorExhaustion);
            assert!(!tracker.observe(outcome), "the failure count never moved");
        }
    }
}
