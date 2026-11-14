use std::time::Duration;

use super::*;
use crate::daemon_get::tests::FakeDaemon;

const OK: &str = include_str!("../../../../../api/internal/core/bootgate/testdata/health-ok.json");
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

#[test]
fn the_three_known_states_map_to_their_health() {
    assert_eq!(
        parse_health(OK.as_bytes()),
        Health::Ready { pid: Some(4242) }
    );
    let body: Value = serde_json::from_str(MIGRATING).unwrap();
    assert_eq!(
        parse_health(MIGRATING.as_bytes()),
        Health::Migrating {
            progress: MigrationProgress::parse(&body["data"]["detail"])
        }
    );
    assert_eq!(
        parse_health(HALTED.as_bytes()),
        Health::Halted {
            reason: Some("layout_newer".into()),
            message: HALT_MESSAGE.into()
        }
    );
}

#[test]
fn a_daemon_that_has_not_reached_its_first_step_is_migrating_without_progress() {
    assert_eq!(
        parse_health(NO_STEP_YET.as_bytes()),
        Health::Migrating { progress: None }
    );
}

#[test]
fn a_halt_without_a_message_still_halts() {
    let body = br#"{"success":true,"data":{"status":"halted"}}"#;
    assert!(matches!(
        parse_health(body),
        Health::Halted { reason: None, .. }
    ));
}

#[test]
fn a_daemon_that_names_no_state_or_an_unknown_one_is_ready() {
    assert_eq!(
        parse_health(br#"{"data":{"pid":7}}"#),
        Health::Ready { pid: Some(7) }
    );
    assert_eq!(
        parse_health(br#"{"data":{"status":"resting","pid":7}}"#),
        Health::Ready { pid: Some(7) }
    );
}

#[test]
fn an_unreadable_body_or_pid_zero_is_ready_without_a_pid() {
    for body in [
        &b"not json"[..],
        br#"{"data":{"status":"ok"}}"#,
        br#"{"data":{"pid":0}}"#,
    ] {
        assert_eq!(parse_health(body), Health::Ready { pid: None });
    }
}

#[tokio::test(start_paused = true)]
async fn a_migrating_daemon_is_waited_for_without_spending_the_budget_and_progress_is_reported_once_per_change()
 {
    let daemon = daemon(vec![NO_STEP_YET, NO_STEP_YET, MIGRATING, MIGRATING, OK]);
    let mut heard = Vec::new();
    let pid = wait_with(&daemon, &timings(1), |progress| {
        heard.push(progress.cloned());
    })
    .await
    .unwrap();
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

#[tokio::test(start_paused = true)]
async fn a_halted_daemon_ends_the_wait_with_its_reason_and_message() {
    let daemon = daemon(vec![MIGRATING, HALTED]);
    let outcome = wait_with(&daemon, &timings(30), |_| {}).await;
    assert!(
        matches!(&outcome, Err(Error::Halted { reason: Some(r), message }) if r == "layout_newer" && message == HALT_MESSAGE),
        "{outcome:?}"
    );
}

#[tokio::test(start_paused = true)]
async fn a_daemon_that_never_answers_runs_the_budget_down() {
    let daemon = FakeDaemon::answering(|_, _| (200, String::new()))
        .refusing(|_| Some(std::io::Error::from(std::io::ErrorKind::NotFound)));
    let outcome = wait_with(&daemon, &timings(2), |_| {}).await;
    assert!(matches!(outcome, Err(Error::GaveUp(2))), "{outcome:?}");
    assert_eq!(daemon.dialled(), 2, "one dial per attempt");
}

#[tokio::test(start_paused = true)]
async fn an_answer_that_is_not_success_counts_against_the_budget() {
    let daemon = FakeDaemon::answering(|_, _| (503, "{}".into()));
    let outcome = wait_with(&daemon, &timings(3), |_| {}).await;
    assert!(matches!(outcome, Err(Error::GaveUp(3))), "{outcome:?}");
}
