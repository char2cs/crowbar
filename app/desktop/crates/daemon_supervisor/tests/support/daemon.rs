use std::time::Duration;

use daemon_supervisor::{DaemonEvent, DaemonPaths, SupervisorConfig, Timings};
use tempfile::TempDir;
use tokio::sync::broadcast::Receiver;

const EVENT_BUDGET: Duration = Duration::from_secs(30);

pub(crate) fn fast_timings() -> Timings {
    Timings {
        health_poll: Duration::from_millis(10),
        health_attempts: 100,
        health_probe_timeout: Duration::from_secs(2),
        watchdog_interval: Duration::from_secs(3600),
        respawn_base: Duration::from_millis(5),
        respawn_cap: Duration::from_millis(20),
        sigterm_grace: Duration::from_secs(5),
        ..Timings::default()
    }
}

/// A supervisor config whose `crowbar-api` is a script that runs the fake daemon in `mode`. The
/// returned directory holds the socket, script and log and is removed when it is dropped.
pub(crate) fn config(mode: &str, timings: Timings) -> (SupervisorConfig, TempDir) {
    let dir = tempfile::Builder::new()
        .prefix("cb-sup-")
        .tempdir()
        .expect("test dir");
    let script = dir.path().join("crowbar-api");
    let fake = env!("CARGO_BIN_EXE_fake_daemon");
    let body = format!("#!/bin/sh\nFAKE_DAEMON_MODE={mode} exec {fake} \"$@\"\n");
    std::fs::write(&script, body).expect("script");
    std::fs::set_permissions(&script, std::os::unix::fs::PermissionsExt::from_mode(0o755))
        .expect("chmod");
    let paths = DaemonPaths {
        socket: dir.path().join("d.sock"),
        log: dir.path().join("logs").join("daemon.log"),
    };
    let mut config = SupervisorConfig::new(script, paths, None, "1.2.3-test");
    config.timings = timings;
    (config, dir)
}

async fn first_matching(
    events: &mut Receiver<DaemonEvent>,
    wanted: impl Fn(&DaemonEvent) -> bool,
) -> DaemonEvent {
    loop {
        let event = events.recv().await.expect("event stream open");
        if wanted(&event) {
            return event;
        }
    }
}

pub(crate) async fn next_matching(
    events: &mut Receiver<DaemonEvent>,
    wanted: impl Fn(&DaemonEvent) -> bool,
) -> DaemonEvent {
    tokio::time::timeout(EVENT_BUDGET, first_matching(events, wanted))
        .await
        .expect("event arrived in time")
}
