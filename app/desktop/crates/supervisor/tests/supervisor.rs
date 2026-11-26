//! Public-API tests of `supervisor` against a fake `crowbar-api` binary.

#[path = "support/real_infra/late_launch.rs"]
mod late_launch;
#[path = "../src/test_support/script.rs"]
mod script;
#[path = "support/real_infra/daemon.rs"]
mod support;

use late_launch::{heard_until, launch_until_startup_is_spent, probing};
use std::path::PathBuf;
use std::time::Duration;
use supervisor::{DaemonEvent, Error, Supervisor, Timings};
use support::{config, fast_timings, next_matching, stop_within_budget};
use tokio::sync::broadcast::Receiver;

type TestResult = Result<(), Box<dyn std::error::Error>>;

fn is_ready(e: &DaemonEvent) -> bool {
    matches!(e, DaemonEvent::Ready { .. })
}

#[tokio::test]
#[ignore = "real-infra: supervises a real fake_daemon child process over a real unix socket"]
async fn a_healthy_daemon_goes_starting_then_ready_and_stops_gracefully() -> TestResult {
    let (cfg, _home) = config("ok", fast_timings())?;
    let (socket, log) = (cfg.paths.socket.clone(), cfg.paths.log.clone());
    let supervisor = Supervisor::start(cfg);
    let mut events = supervisor.events();
    let ready = next_matching(&mut events, is_ready).await?;
    assert!(matches!(ready, DaemonEvent::Ready { pid: Some(_) }));
    assert!(socket.exists());
    assert!(matches!(
        supervisor.restart().await,
        Err(Error::AlreadyRunning)
    ));
    let mut stopped = supervisor.events();
    supervisor.stop().await;
    assert_eq!(
        next_matching(&mut stopped, |e| *e == DaemonEvent::Stopped).await?,
        DaemonEvent::Stopped
    );
    let text = std::fs::read_to_string(log).expect("log written");
    assert!(
        text.contains("=== daemon spawned")
            && text.contains("=== daemon terminated")
            && text.contains("SIGTERM"),
        "{text}"
    );
    Ok(())
}

#[tokio::test]
#[ignore = "real-infra: supervises a real fake_daemon child process over a real unix socket"]
async fn a_daemon_that_halts_is_reported_and_not_respawned_until_asked() -> TestResult {
    let (cfg, _home) = config("halted", fast_timings())?;
    let supervisor = Supervisor::start(cfg);
    let mut events = supervisor.events();
    let halted = next_matching(&mut events, |e| matches!(e, DaemonEvent::Halted { .. })).await?;
    assert_eq!(
        halted,
        DaemonEvent::Halted {
            reason: Some("layout_newer".into()),
            message: "layout too new".into()
        }
    );
    supervisor
        .restart()
        .await
        .expect("restart accepted while halted");
    next_matching(&mut events, |e| *e == DaemonEvent::Restarted).await?;
    supervisor.stop().await;
    Ok(())
}

#[tokio::test]
#[ignore = "real-infra: supervises a real fake_daemon child process over a real unix socket"]
async fn a_crash_loop_backs_off_then_gives_up_with_a_report_and_a_manual_restart_gets_a_fresh_budget()
-> TestResult {
    let (cfg, _home) = config("crash", fast_timings())?;
    let supervisor = Supervisor::start(cfg);
    let mut events = supervisor.events();
    for attempt in 1..=3 {
        let event =
            next_matching(&mut events, |e| matches!(e, DaemonEvent::Recovering { .. })).await?;
        assert!(
            matches!(event, DaemonEvent::Recovering { attempt: a, .. } if a == attempt),
            "{event:?}"
        );
    }
    let DaemonEvent::GaveUp { report } =
        next_matching(&mut events, |e| matches!(e, DaemonEvent::GaveUp { .. })).await?
    else {
        unreachable!("filtered to GaveUp");
    };
    assert!(report.contains("exited 4 times"), "{report}");
    assert!(report.starts_with("Crowbar 1.2.3-test on "), "{report}");
    supervisor
        .restart()
        .await
        .expect("restart accepted after giving up");
    let again = next_matching(&mut events, |e| matches!(e, DaemonEvent::Recovering { .. })).await?;
    assert!(
        matches!(again, DaemonEvent::Recovering { attempt: 1, .. }),
        "{again:?}"
    );
    supervisor.stop().await;
    Ok(())
}

#[tokio::test]
#[ignore = "real-infra: supervises a real fake_daemon child process over a real unix socket"]
async fn a_daemon_that_dies_after_becoming_ready_is_recovered_and_comes_back() -> TestResult {
    let (cfg, _home) = config("die", fast_timings())?;
    let supervisor = Supervisor::start(cfg);
    let mut events = supervisor.events();
    next_matching(&mut events, is_ready).await?;
    next_matching(&mut events, |e| matches!(e, DaemonEvent::Recovering { .. })).await?;
    next_matching(&mut events, is_ready).await?;
    supervisor.stop().await;
    Ok(())
}

#[tokio::test]
#[ignore = "real-infra: supervises a real fake_daemon child process over a real unix socket"]
async fn a_launch_that_cannot_run_or_exits_is_charged_with_the_reason_named() -> TestResult {
    let cases = [
        (
            "missing binary",
            "ok",
            Some("/definitely/not/crowbar-api"),
            "cannot run",
        ),
        ("exits at once", "crash", None, "daemon terminated"),
    ];
    for (name, mode, binary, reason) in cases {
        let (mut cfg, _home) = config(mode, fast_timings())?;
        cfg.binary = binary.map_or(cfg.binary, PathBuf::from);
        let supervisor = Supervisor::start(cfg);
        let mut events = supervisor.events();
        let recovering = |e: &DaemonEvent| matches!(e, DaemonEvent::Recovering { .. });
        let event = next_matching(&mut events, recovering).await?;
        supervisor.stop().await;
        assert!(
            matches!(&event, DaemonEvent::Recovering { attempt: 1, reason: said, .. } if said.contains(reason)),
            "{name}: {event:?}"
        );
    }
    Ok(())
}

#[tokio::test]
#[ignore = "real-infra: supervises a real fake_daemon child process over a real unix socket"]
async fn a_daemon_that_ignores_sigterm_is_killed_once_the_grace_runs_out() -> TestResult {
    let timings = Timings {
        sigterm_grace: Duration::from_millis(200),
        ..fast_timings()
    };
    let (cfg, _home) = config("deaf", timings)?;
    let log = cfg.paths.log.clone();
    let supervisor = Supervisor::start(cfg);
    let mut events = supervisor.events();
    next_matching(&mut events, is_ready).await?;
    tokio::time::timeout(Duration::from_secs(30), supervisor.stop())
        .await
        .expect("stop returned");
    let text = std::fs::read_to_string(log).expect("log written");
    assert!(text.contains("SIGKILL"), "{text}");
    Ok(())
}

#[tokio::test]
#[ignore = "real-infra: supervises a real fake_daemon child process over a real unix socket"]
async fn the_watchdog_kills_a_daemon_that_stops_answering_and_it_is_recovered() -> TestResult {
    let timings = Timings {
        watchdog_interval: Duration::from_millis(20),
        probe_timeout: Duration::from_millis(300),
        grace_probe_timeout: Duration::from_millis(300),
        failure_threshold: 2,
        dump_timeout: Duration::from_millis(100),
        sigquit_grace: Duration::from_secs(5),
        ..fast_timings()
    };
    let (cfg, _home) = config("stall", timings)?;
    let supervisor = Supervisor::start(cfg);
    let mut events = supervisor.events();
    next_matching(&mut events, is_ready).await?;
    next_matching(&mut events, |e| *e == DaemonEvent::Unhealthy).await?;
    next_matching(&mut events, |e| matches!(e, DaemonEvent::Recovering { .. })).await?;
    supervisor.stop().await;
    Ok(())
}

type Step = (String, u32, u32);

async fn heard_until_ready(
    events: &mut Receiver<DaemonEvent>
) -> Result<Vec<DaemonEvent>, Box<dyn std::error::Error>> {
    let mut heard = Vec::new();
    while !heard.last().is_some_and(is_ready) {
        heard.push(next_matching(events, |_| true).await?);
    }
    Ok(heard)
}

#[tokio::test]
#[ignore = "real-infra: supervises a real fake_daemon child process over a real unix socket"]
async fn a_migrating_daemon_is_reported_step_by_step_and_then_ready() -> TestResult {
    let (cfg, _home) = config("steps", fast_timings())?;
    let supervisor = Supervisor::start(cfg);
    let mut events = supervisor.events();
    let heard = heard_until_ready(&mut events).await?;
    supervisor.stop().await;
    let steps: Vec<Option<Step>> = heard
        .iter()
        .filter_map(|event| match event {
            DaemonEvent::Migrating { progress } => Some(
                progress
                    .as_ref()
                    .map(|p| (p.step().to_owned(), p.index(), p.total())),
            ),
            _ => None,
        })
        .collect();
    assert_eq!(
        steps,
        [
            None,
            Some(("first".into(), 1, 2)),
            Some(("second".into(), 2, 2))
        ],
        "{heard:?}"
    );
    Ok(())
}

#[tokio::test]
#[ignore = "real-infra: supervises a real fake_daemon child process over a real unix socket"]
async fn a_daemon_that_prints_bytes_that_are_not_utf8_never_wedges_on_a_full_pipe() -> TestResult {
    let (cfg, _home) = config("noisy", fast_timings())?;
    let log = cfg.paths.log.clone();
    let supervisor = Supervisor::start(cfg);
    let mut events = supervisor.events();
    next_matching(&mut events, is_ready).await?;
    supervisor.stop().await;
    let text = std::fs::read_to_string(log).expect("log written");
    assert!(
        text.contains("[err] \u{fffd}"),
        "the noise was logged lossily"
    );
    assert!(
        text.len() < 8 * (1 << 20),
        "and not unboundedly: {} bytes",
        text.len()
    );
    Ok(())
}

const NEVER_KILLED: u32 = 1000;
const KILLED_AFTER: u32 = 2;

#[derive(Debug, Clone, Copy)]
enum HeardBy {
    Shell,
    Watchdog,
}

fn tell(
    heard_by: HeardBy,
    supervisor: &Supervisor,
) {
    if matches!(heard_by, HeardBy::Watchdog) {
        return;
    }
    supervisor.heard_answer();
}

fn charged(event: &DaemonEvent) -> bool {
    matches!(
        event,
        DaemonEvent::Recovering { .. } | DaemonEvent::Restarted
    )
}

#[tokio::test]
#[ignore = "real-infra: supervises a real fake_daemon child process over a real unix socket"]
async fn a_late_answer_makes_the_launch_ready_without_a_respawn() -> TestResult {
    let cases = [
        (HeardBy::Shell, fast_timings()),
        (HeardBy::Watchdog, probing(NEVER_KILLED)),
    ];
    for (heard_by, timings) in cases {
        let mut launch = launch_until_startup_is_spent(timings).await?;
        launch.let_serve()?;
        tell(heard_by, &launch.supervisor);
        let ready = |e: &DaemonEvent| matches!(e, DaemonEvent::Ready { pid: Some(_) });
        let heard = heard_until(&mut launch.events, ready).await?;
        stop_within_budget(launch.supervisor).await?;
        assert!(!heard.iter().any(charged), "{heard_by:?}: {heard:?}");
    }
    Ok(())
}

#[tokio::test]
#[ignore = "real-infra: supervises a real fake_daemon child process over a real unix socket"]
async fn a_launch_that_never_serves_is_killed_by_the_watchdog_and_charged_once() -> TestResult {
    let mut launch = launch_until_startup_is_spent(probing(KILLED_AFTER)).await?;
    let recovering = |e: &DaemonEvent| matches!(e, DaemonEvent::Recovering { .. });
    let heard = heard_until(&mut launch.events, recovering).await?;
    stop_within_budget(launch.supervisor).await?;
    let tail: Vec<_> = heard
        .iter()
        .skip_while(|e| **e != DaemonEvent::Unhealthy)
        .collect();
    assert!(
        matches!(
            tail.as_slice(),
            [
                DaemonEvent::Unhealthy,
                DaemonEvent::Recovering { attempt: 1, .. }
            ]
        ),
        "{heard:?}"
    );
    Ok(())
}
