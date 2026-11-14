//! Public-API tests of `daemon_supervisor` against a fake `crowbar-api` binary.
#![allow(
    clippy::expect_used,
    clippy::unwrap_used,
    clippy::indexing_slicing,
    clippy::string_slice,
    clippy::excessive_nesting,
    reason = "test code: a failing fake or assertion should fail the test loudly, and async blocks nest by construction"
)]

mod support;

use daemon_supervisor::{DaemonEvent, Error, Supervisor, Timings};
use std::time::Duration;
use support::{config, fast_timings, log_of, next_matching};

fn is_ready(e: &DaemonEvent) -> bool {
    matches!(e, DaemonEvent::Ready { .. })
}

#[tokio::test]
async fn a_healthy_daemon_goes_starting_then_ready_and_stops_gracefully() {
    let cfg = config("ok", "ok", fast_timings());
    let (socket, log) = (cfg.paths.socket.clone(), log_of(&cfg));
    let supervisor = Supervisor::start(cfg);
    let mut events = supervisor.events();
    let ready = next_matching(&mut events, is_ready).await;
    assert!(matches!(ready, DaemonEvent::Ready { pid: Some(_) }));
    assert!(socket.exists());
    assert!(matches!(
        supervisor.restart().await,
        Err(Error::AlreadyRunning)
    ));
    let mut stopped = supervisor.events();
    supervisor.stop().await;
    assert_eq!(
        next_matching(&mut stopped, |e| *e == DaemonEvent::Stopped).await,
        DaemonEvent::Stopped
    );
    let text = std::fs::read_to_string(log).expect("log written");
    assert!(
        text.contains("=== daemon spawned") && text.contains("=== daemon terminated"),
        "{text}"
    );
}

#[tokio::test]
async fn a_daemon_that_halts_is_reported_and_not_respawned_until_asked() {
    let supervisor = Supervisor::start(config("halted", "halted", fast_timings()));
    let mut events = supervisor.events();
    let halted = next_matching(&mut events, |e| matches!(e, DaemonEvent::Halted { .. })).await;
    assert_eq!(
        halted,
        DaemonEvent::Halted {
            message: "layout too new".into()
        }
    );
    supervisor
        .restart()
        .await
        .expect("restart accepted while halted");
    next_matching(&mut events, |e| *e == DaemonEvent::Starting).await;
    supervisor.stop().await;
}

#[tokio::test]
async fn a_crash_loop_backs_off_then_gives_up_with_a_report_and_a_manual_restart_gets_a_fresh_budget()
 {
    let supervisor = Supervisor::start(config("crash", "crash", fast_timings()));
    let mut events = supervisor.events();
    for attempt in 1..=3 {
        let event =
            next_matching(&mut events, |e| matches!(e, DaemonEvent::Recovering { .. })).await;
        assert!(
            matches!(event, DaemonEvent::Recovering { attempt: a, .. } if a == attempt),
            "{event:?}"
        );
    }
    let DaemonEvent::GaveUp { report } =
        next_matching(&mut events, |e| matches!(e, DaemonEvent::GaveUp { .. })).await
    else {
        unreachable!("filtered to GaveUp");
    };
    assert!(report.contains("exited 4 times"), "{report}");
    supervisor
        .restart()
        .await
        .expect("restart accepted after giving up");
    let again = next_matching(&mut events, |e| matches!(e, DaemonEvent::Recovering { .. })).await;
    assert!(
        matches!(again, DaemonEvent::Recovering { attempt: 1, .. }),
        "{again:?}"
    );
    supervisor.stop().await;
}

#[tokio::test]
async fn a_daemon_that_dies_after_becoming_ready_is_recovered_and_comes_back() {
    let supervisor = Supervisor::start(config("die", "die", fast_timings()));
    let mut events = supervisor.events();
    next_matching(&mut events, is_ready).await;
    next_matching(&mut events, |e| matches!(e, DaemonEvent::Recovering { .. })).await;
    next_matching(&mut events, is_ready).await;
    supervisor.stop().await;
}

#[tokio::test]
async fn a_daemon_that_never_answers_is_stopped_and_recovered() {
    let timings = Timings {
        health_attempts: 3,
        health_probe_timeout: Duration::from_millis(50),
        ..fast_timings()
    };
    let supervisor = Supervisor::start(config("never", "wedge", timings));
    let mut events = supervisor.events();
    let event = next_matching(&mut events, |e| matches!(e, DaemonEvent::Recovering { .. })).await;
    assert!(
        matches!(&event, DaemonEvent::Recovering { reason, .. } if reason.contains("never became healthy")),
        "{event:?}"
    );
    supervisor.stop().await;
}

#[tokio::test]
async fn the_watchdog_kills_a_daemon_that_stops_answering_and_it_is_recovered() {
    let timings = Timings {
        watchdog_interval: Duration::from_millis(20),
        probe_timeout: Duration::from_millis(100),
        grace_probe_timeout: Duration::from_millis(100),
        failure_threshold: 2,
        dump_timeout: Duration::from_millis(100),
        sigquit_grace: Duration::from_secs(5),
        ..fast_timings()
    };
    let supervisor = Supervisor::start(config("stall", "stall", timings));
    let mut events = supervisor.events();
    next_matching(&mut events, is_ready).await;
    next_matching(&mut events, |e| *e == DaemonEvent::Unhealthy).await;
    next_matching(&mut events, |e| matches!(e, DaemonEvent::Recovering { .. })).await;
    supervisor.stop().await;
}

#[tokio::test]
async fn a_missing_binary_is_recovered_then_reported_not_panicked_on() {
    let mut cfg = config("missing", "ok", fast_timings());
    cfg.binary = "/definitely/not/crowbar-api".into();
    let supervisor = Supervisor::start(cfg);
    let mut events = supervisor.events();
    let event = next_matching(&mut events, |e| matches!(e, DaemonEvent::Recovering { .. })).await;
    assert!(
        matches!(&event, DaemonEvent::Recovering { reason, .. } if reason.contains("cannot run")),
        "{event:?}"
    );
    supervisor.stop().await;
}
