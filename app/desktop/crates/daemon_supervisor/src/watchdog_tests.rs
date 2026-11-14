use super::*;
use crate::bus::Bus;
use crate::timings::Timings;
use std::sync::Arc;
use tokio::sync::mpsc;
use tokio_util::sync::CancellationToken;

fn context(
    socket: std::path::PathBuf,
    cancel: CancellationToken,
    wedged: mpsc::Sender<()>,
) -> WatchContext {
    let timings = Timings {
        watchdog_interval: Duration::from_millis(1),
        probe_timeout: Duration::from_millis(50),
        grace_probe_timeout: Duration::from_millis(50),
        failure_threshold: 2,
        ..Timings::default()
    };
    WatchContext {
        socket,
        pid: None,
        timings,
        bus: Arc::new(Bus::new()),
        wedged,
        cancel,
    }
}

#[tokio::test]
async fn cancelling_ends_the_watch() {
    let (wedged, _rx) = mpsc::channel(1);
    let cancel = CancellationToken::new();
    cancel.cancel();
    watch(context("/nope.sock".into(), cancel, wedged)).await;
}

#[tokio::test]
async fn a_daemon_that_cannot_be_dialled_is_reported_wedged_after_the_threshold_with_no_pid_to_exonerate_it()
 {
    let (wedged, mut asked) = mpsc::channel(1);
    let cancel = CancellationToken::new();
    let ctx = context("/definitely/absent.sock".into(), cancel.clone(), wedged);
    let bus = Arc::clone(&ctx.bus);
    let mut events = bus.subscribe();
    let task = tokio::spawn(watch(ctx));
    tokio::time::timeout(Duration::from_secs(30), asked.recv())
        .await
        .unwrap()
        .unwrap();
    assert_eq!(events.recv().await.unwrap(), DaemonEvent::Unhealthy);
    cancel.cancel();
    task.await.unwrap();
}

async fn hold_connections_open(listener: tokio::net::UnixListener) {
    let mut open = Vec::new();
    while let Ok((stream, _)) = listener.accept().await {
        open.push(stream);
    }
}

#[tokio::test(start_paused = true)]
async fn a_cycle_that_is_still_probing_when_the_watch_is_cancelled_reports_nothing() {
    let dir = tempfile::tempdir().unwrap();
    let socket = dir.path().join("silent.sock");
    let listener = tokio::net::UnixListener::bind(&socket).unwrap();
    let held = tokio::spawn(hold_connections_open(listener));
    let (wedged, mut asked) = mpsc::channel(1);
    let cancel = CancellationToken::new();
    let mut ctx = context(socket, cancel.clone(), wedged);
    ctx.timings.failure_threshold = 1;
    ctx.timings.watchdog_interval = Duration::from_secs(1);
    ctx.timings.probe_timeout = Duration::from_secs(5);
    ctx.timings.grace_probe_timeout = Duration::from_secs(5);
    let mut events = ctx.bus.subscribe();
    let task = tokio::spawn(watch(ctx));
    tokio::time::sleep(Duration::from_secs(3)).await;
    cancel.cancel();
    task.await.unwrap();
    tokio::time::sleep(Duration::from_secs(60)).await;
    assert!(
        events.try_recv().is_err(),
        "no Unhealthy for a daemon that is gone"
    );
    assert!(asked.try_recv().is_err(), "no kill request either");
    held.abort();
}
