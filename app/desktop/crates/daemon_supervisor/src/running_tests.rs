use std::process::Stdio;

use tokio::process::Command;

use super::*;

#[tokio::test]
async fn dropping_a_running_daemon_stops_its_watchdog() {
    let child = Command::new("/bin/sh")
        .args(["-c", "exit 0"])
        .stdin(Stdio::null())
        .spawn()
        .unwrap();
    let (_wedged, wedged) = mpsc::channel(1);
    let watchdog = CancellationToken::new();
    let running = Running {
        child,
        wedged,
        watchdog: watchdog.clone(),
    };
    assert!(!watchdog.is_cancelled());
    drop(running);
    assert!(watchdog.is_cancelled());
}
