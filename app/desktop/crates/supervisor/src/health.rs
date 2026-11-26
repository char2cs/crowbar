mod announced;
mod attempt;
pub(crate) mod client;
mod health_event;
mod migration_progress;
mod probe;

pub use client::fetch;
pub(crate) use client::{SocketDial, get};
pub use migration_progress::MigrationProgress;
pub(crate) use probe::{Probe, probe_ready};

use std::path::Path;

use serde_json::Value;

use crate::config::Timings;
use crate::error::Error;
use crate::health::announced::Announced;
use crate::health::attempt::Attempt;
use crate::health::client::Dial;
use crate::health::health_event::HealthEvent;

const HEALTH_BODY_LIMIT: usize = 1024 * 1024;
const HEALTH_TARGET: &str = "/v0/health";

#[derive(Debug, Clone, PartialEq, Eq)]
pub(crate) enum Health {
    /// The daemon serves, and says which process it is.
    Ready {
        /// The daemon's self-reported pid.
        pid: Option<u32>,
    },
    /// The daemon is preparing its home.
    Migrating {
        /// The current step, `None` before the first one.
        progress: Option<MigrationProgress>,
    },
    /// The daemon refuses to serve, and says why.
    Halted {
        /// The machine-readable halt reason, `data.detail.reason`.
        reason: Option<String>,
        /// The halt text, `data.detail.message`.
        message: String,
    },
}

#[must_use]
pub(crate) fn parse_health(body: &[u8]) -> Health {
    let data = serde_json::from_slice::<Value>(body)
        .ok()
        .and_then(|v| v.get("data").cloned());
    let field = |name: &str| data.as_ref().and_then(|d| d.get(name));
    let detail_text = |name: &str| {
        field("detail")
            .and_then(|d| d.get(name))
            .and_then(Value::as_str)
            .map(str::to_owned)
    };
    match field("status").and_then(Value::as_str) {
        Some("migrating") => Health::Migrating {
            progress: field("detail").and_then(MigrationProgress::parse),
        },
        Some("halted") => Health::Halted {
            reason: detail_text("reason"),
            message: detail_text("message")
                .unwrap_or_else(|| "the daemon halted without saying why".to_owned()),
        },
        _ => Health::Ready {
            pid: field("pid")
                .and_then(Value::as_u64)
                .and_then(|pid| u32::try_from(pid).ok())
                .filter(|pid| *pid > 0),
        },
    }
}

pub(crate) async fn check_health<D: Dial>(dial: &D) -> Result<Health, Error> {
    get(dial, HEALTH_TARGET, HEALTH_BODY_LIMIT)
        .await
        .map(|body| parse_health(&body))
}

pub(crate) async fn wait_for_health(
    socket: &Path,
    timings: &Timings,
    on_migrating: impl FnMut(Option<&MigrationProgress>) + Send,
) -> Result<Option<u32>, Error> {
    wait_with(&SocketDial::new(socket), timings, on_migrating).await
}

pub(crate) async fn wait_with<D: Dial>(
    dial: &D,
    timings: &Timings,
    mut on_migrating: impl FnMut(Option<&MigrationProgress>) + Send,
) -> Result<Option<u32>, Error> {
    let attempts = timings.health_attempts;
    let mut failed = 0;
    let mut announce = Announced::default();
    loop {
        tokio::time::sleep(timings.health_poll).await;
        match attempt(dial, timings, &mut announce, &mut on_migrating).await {
            Attempt::Served(pid) => return Ok(pid),
            Attempt::Refused(error) => return Err(error),
            Attempt::Waiting => {},
            Attempt::Failed if failed + 1 >= attempts => return Err(Error::GaveUp(attempts)),
            Attempt::Failed => failed += 1,
        }
    }
}

async fn attempt<D: Dial>(
    dial: &D,
    timings: &Timings,
    announce: &mut Announced,
    on_migrating: &mut impl FnMut(Option<&MigrationProgress>),
) -> Attempt {
    match tokio::time::timeout(timings.health_probe_timeout, check_health(dial)).await {
        Ok(Ok(Health::Ready { pid })) => Attempt::Served(pid),
        Ok(Ok(Health::Halted { reason, message })) => {
            Attempt::Refused(Error::Halted { reason, message })
        },
        Ok(Ok(Health::Migrating { progress })) => {
            announce.report(progress, on_migrating);
            Attempt::Waiting
        },
        Ok(Err(error)) => {
            crate::journal_sink::emit(&HealthEvent::Unreadable { error: &error });
            Attempt::Failed
        },
        Err(elapsed) => {
            let after = timings.health_probe_timeout;
            crate::journal_sink::emit(&HealthEvent::Unanswered { after, elapsed });
            Attempt::Failed
        },
    }
}

#[cfg(test)]
mod tests {
    use std::time::Duration;

    use super::*;
    use crate::test_support::fake_daemon::FakeDaemon;

    const OK: &str =
        include_str!("../../../../../api/internal/core/bootgate/testdata/health-ok.json");
    const MIGRATING: &str =
        include_str!("../../../../../api/internal/core/bootgate/testdata/health-migrating.json");
    const HALTED: &str =
        include_str!("../../../../../api/internal/core/bootgate/testdata/health-halted.json");
    const HALT_MESSAGE: &str =
        "this home is at layout 9 but this Crowbar only knows layout 0: run a newer Crowbar";
    const NO_STEP_YET: &str = r#"{"success":true,"data":{"status":"migrating","detail":null}}"#;

    fn timings(attempts: u32) -> Timings {
        Timings {
            health_poll: Duration::from_millis(1),
            health_attempts: attempts,
            health_probe_timeout: Duration::from_secs(2),
            ..Timings::default()
        }
    }

    /// A daemon whose nth health answer is `bodies[n]`, the last one repeating.
    fn daemon(bodies: Vec<&'static str>) -> FakeDaemon {
        FakeDaemon::answering(move |index, _| {
            let last = bodies.len().saturating_sub(1);
            (
                200,
                bodies
                    .get(index.min(last))
                    .copied()
                    .unwrap_or("{}")
                    .to_owned(),
            )
        })
    }

    type HealthCase = (&'static str, &'static [u8], Health);

    fn served_cases() -> Vec<HealthCase> {
        let step: Value = serde_json::from_str(MIGRATING).unwrap();
        let ready = |pid| Health::Ready { pid };
        vec![
            ("ok", OK.as_bytes(), ready(Some(4242))),
            (
                "migrating at a step",
                MIGRATING.as_bytes(),
                Health::Migrating {
                    progress: MigrationProgress::parse(&step["data"]["detail"]),
                },
            ),
            (
                "halted",
                HALTED.as_bytes(),
                Health::Halted {
                    reason: Some("layout_newer".into()),
                    message: HALT_MESSAGE.into(),
                },
            ),
            (
                "migrating before the first step",
                NO_STEP_YET.as_bytes(),
                Health::Migrating { progress: None },
            ),
            (
                "halted without a message",
                br#"{"success":true,"data":{"status":"halted"}}"#,
                Health::Halted {
                    reason: None,
                    message: "the daemon halted without saying why".into(),
                },
            ),
        ]
    }

    fn unnamed_cases() -> Vec<HealthCase> {
        let ready = |pid| Health::Ready { pid };
        vec![
            ("no state", br#"{"data":{"pid":7}}"#, ready(Some(7))),
            (
                "unknown state",
                br#"{"data":{"status":"resting","pid":7}}"#,
                ready(Some(7)),
            ),
            ("unreadable body", b"not json", ready(None)),
            ("no pid", br#"{"data":{"status":"ok"}}"#, ready(None)),
            ("pid zero", br#"{"data":{"pid":0}}"#, ready(None)),
        ]
    }

    #[test]
    fn every_health_body_maps_to_its_state() {
        for (name, body, expected) in served_cases().into_iter().chain(unnamed_cases()) {
            assert_eq!(parse_health(body), expected, "{name}");
        }
    }

    fn ignore(_: Option<&MigrationProgress>) {}

    async fn heard_progress(daemon: &FakeDaemon) -> (Option<u32>, Vec<Option<MigrationProgress>>) {
        let mut heard = Vec::new();
        let pid = wait_with(daemon, &timings(1), |progress| {
            heard.push(progress.cloned());
        })
        .await
        .unwrap();
        (pid, heard)
    }

    #[tokio::test(start_paused = true)]
    async fn a_migrating_daemon_is_waited_for_without_spending_the_budget_and_progress_is_reported_once_per_change()
     {
        let daemon = daemon(vec![NO_STEP_YET, NO_STEP_YET, MIGRATING, MIGRATING, OK]);
        let (pid, heard) = heard_progress(&daemon).await;
        assert_eq!(pid, Some(4242));
        assert_eq!(heard.len(), 2, "no step, then the step: {heard:?}");
        assert_eq!(heard.first(), Some(&None), "starting");
        assert_eq!(
            heard
                .get(1)
                .and_then(Option::as_ref)
                .map(MigrationProgress::step),
            Some("workspace-paths-table")
        );
    }

    fn halted_on_a_newer_layout(outcome: &Result<Option<u32>, Error>) -> bool {
        matches!(outcome, Err(Error::Halted { reason, message })
            if reason.as_deref() == Some("layout_newer") && message == HALT_MESSAGE)
    }

    async fn assert_gave_up(cases: [(&str, FakeDaemon, u32); 2]) {
        for (name, daemon, attempts) in cases {
            let outcome = wait_with(&daemon, &timings(attempts), ignore).await;
            let gave_up = matches!(outcome, Err(Error::GaveUp(spent)) if spent == attempts);
            assert!(gave_up, "{name}: {outcome:?}");
            let dials = usize::try_from(attempts).unwrap();
            assert_eq!(daemon.dialled(), dials, "{name}: one dial per attempt");
        }
    }

    #[tokio::test(start_paused = true)]
    async fn a_daemon_that_halts_or_never_answers_well_ends_the_wait_with_the_reason() {
        let halted = wait_with(&daemon(vec![MIGRATING, HALTED]), &timings(30), ignore).await;
        assert!(halted_on_a_newer_layout(&halted), "{halted:?}");
        let unreachable = FakeDaemon::answering(|_, _| (200, String::new()))
            .refusing(|_| Some(std::io::Error::from(std::io::ErrorKind::NotFound)));
        let not_success = FakeDaemon::answering(|_, _| (503, "{}".into()));
        let cases = [
            ("never answers", unreachable, 2),
            ("not success", not_success, 3),
        ];
        assert_gave_up(cases).await;
    }
}
