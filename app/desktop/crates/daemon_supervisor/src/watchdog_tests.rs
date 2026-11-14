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
