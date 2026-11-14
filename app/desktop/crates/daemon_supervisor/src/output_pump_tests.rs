use std::process::Stdio;

use tokio::process::Command;

use super::*;

#[tokio::test]
async fn stdout_and_stderr_land_in_the_log_with_their_prefixes() {
    let path = std::env::temp_dir().join(format!("cb-pump-{}.log", std::process::id()));
    std::fs::remove_file(&path).ok();
    let log = Arc::new(DaemonLog::open(&path, 1 << 20).unwrap());
    let mut child = Command::new("/bin/sh")
        .args(["-c", "echo to-out; echo to-err >&2"])
        .stdout(Stdio::piped())
        .stderr(Stdio::piped())
        .spawn()
        .unwrap();
    capture(&mut child, &log);
    child.wait().await.unwrap();
    let deadline = tokio::time::Instant::now() + std::time::Duration::from_secs(10);
    while !holds_both(&path) {
        assert!(tokio::time::Instant::now() < deadline, "log never held both lines");
        tokio::task::yield_now().await;
    }
}

fn holds_both(path: &std::path::Path) -> bool {
    let text = std::fs::read_to_string(path).unwrap_or_default();
    text.contains("[out] to-out") && text.contains("[err] to-err")
}
