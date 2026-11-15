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

/// Deep readiness probe: `/v0/projects` goes through the view store every observed wedge pinned,
/// unlike the static `/v0/health`. Every dial is classified, not just the first: a process out of
/// descriptors cannot reach even a healthy daemon, whichever request ran out.
pub async fn probe_ready(socket: &Path) -> Probe {
    probe_with(&SocketDial::new(socket)).await
}

#[cfg(test)]
mod tests {
    use std::io::ErrorKind;

    use rustix::io::Errno;

    use super::*;
    use crate::failure_tracker::FailureTracker;
    use crate::test_support::fake_daemon::FakeDaemon;

    const OK: &str =
        include_str!("../../../../../api/internal/core/bootgate/testdata/health-ok.json");
    const MIGRATING: &str =
        include_str!("../../../../../api/internal/core/bootgate/testdata/health-migrating.json");
    const HALTED: &str =
        include_str!("../../../../../api/internal/core/bootgate/testdata/health-halted.json");
    const THRESHOLD: u32 = 6;

    fn answer_for(
        target: &str,
        health: &str,
        other: u16,
    ) -> (u16, String) {
        if target != "/v0/health" {
            return (other, "{}".to_owned());
        }
        (200, health.to_owned())
    }

    fn daemon(
        health: &'static str,
        other: u16,
    ) -> FakeDaemon {
        FakeDaemon::answering(move |_, target| answer_for(target, health, other))
    }

    async fn watched(daemon: &FakeDaemon) -> (Vec<Probe>, bool) {
        let mut tracker = FailureTracker::new(THRESHOLD);
        let (mut outcomes, mut tripped) = (Vec::new(), false);
        for _ in 0..THRESHOLD * 2 {
            let outcome = probe_with(daemon).await;
            tripped |= tracker.observe(outcome);
            outcomes.push(outcome);
        }
        (outcomes, tripped)
    }

    fn out_of_descriptors(errno: Errno) -> std::io::Error {
        std::io::Error::from_raw_os_error(errno.raw_os_error())
    }

    async fn assert_held(cases: [(&str, &'static str); 2]) {
        for (tag, health) in cases {
            let (outcomes, tripped) = watched(&daemon(health, 503)).await;
            assert!(
                outcomes.iter().all(|o| *o == Probe::Held),
                "{tag}: {outcomes:?}"
            );
            assert!(!tripped, "{tag}");
        }
    }

    #[tokio::test]
    async fn a_migrating_or_halted_daemon_is_held_and_never_killed() {
        assert_held([("migrating", MIGRATING), ("halted", HALTED)]).await;
    }

    #[tokio::test]
    async fn a_serving_daemon_whose_deep_probe_fails_is_still_indicted() {
        let (outcomes, tripped) = watched(&daemon(OK, 503)).await;
        assert!(
            outcomes.iter().all(|o| *o == Probe::Unserved),
            "{outcomes:?}"
        );
        assert!(tripped);
    }

    #[tokio::test]
    async fn a_serving_daemon_that_answers_is_healthy() {
        let (outcomes, tripped) = watched(&daemon(OK, 200)).await;
        assert!(
            outcomes.iter().all(|o| *o == Probe::Healthy),
            "{outcomes:?}"
        );
        assert!(!tripped);
    }

    #[tokio::test]
    async fn a_probe_dials_once_per_request_and_not_an_extra_time() {
        let daemon = daemon(OK, 200);
        assert_eq!(probe_with(&daemon).await, Probe::Healthy);
        assert_eq!(daemon.dialled(), 2, "health, then projects");
    }

    #[tokio::test]
    async fn running_out_of_descriptors_on_any_dial_is_never_blamed_on_the_daemon() {
        let cases = [
            (0, Errno::MFILE),
            (0, Errno::NFILE),
            (1, Errno::MFILE),
            (1, Errno::NFILE),
        ];
        assert_exhaustion_blamed_on_us(cases).await;
    }

    async fn assert_exhaustion_blamed_on_us(cases: [(usize, Errno); 4]) {
        for (nth, errno) in cases {
            let daemon =
                daemon(OK, 200).refusing(move |n| (n == nth).then(|| out_of_descriptors(errno)));
            assert_eq!(
                probe_with(&daemon).await,
                Probe::LocalDescriptorExhaustion,
                "dial {nth} failed with {errno:?}"
            );
        }
    }

    #[tokio::test]
    async fn a_healthy_daemon_is_never_indicted_however_often_a_dial_runs_dry() {
        let daemon =
            daemon(OK, 200).refusing(|n| (n % 2 == 1).then(|| out_of_descriptors(Errno::MFILE)));
        let (outcomes, tripped) = watched(&daemon).await;
        assert!(
            outcomes
                .iter()
                .all(|o| *o == Probe::LocalDescriptorExhaustion),
            "{outcomes:?}"
        );
        assert!(!tripped, "the watchdog's failure count never moved");
    }

    #[tokio::test]
    async fn a_refused_connection_is_a_daemon_that_does_not_serve() {
        let daemon = daemon(OK, 200).refusing(|_| Some(out_of_descriptors(Errno::CONNREFUSED)));
        assert_eq!(probe_with(&daemon).await, Probe::Unserved);
        let missing = FakeDaemon::answering(|_, _| (200, String::new()))
            .refusing(|_| Some(std::io::Error::from(ErrorKind::NotFound)));
        assert_eq!(probe_with(&missing).await, Probe::Unserved);
    }
}
