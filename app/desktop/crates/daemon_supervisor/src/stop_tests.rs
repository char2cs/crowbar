use std::process::Stdio;

use tokio::process::Command;

use super::*;

fn shell(script: &str) -> Child {
    Command::new("/bin/sh")
        .args(["-c", script])
        .stdin(Stdio::null())
        .kill_on_drop(true)
        .spawn()
        .unwrap()
}

#[tokio::test]
async fn a_process_that_honours_the_first_signal_is_not_waited_out() {
    let mut child = shell("exec sleep 30");
    let status = stop_with(&mut child, Signal::TERM, Duration::from_secs(60))
        .await
        .unwrap();
    assert!(!status.success());
}

#[tokio::test(start_paused = true)]
async fn a_process_that_ignores_the_first_signal_is_killed_after_the_grace() {
    let mut child = shell("trap '' TERM; while :; do sleep 1; done");
    let status = stop_with(&mut child, Signal::TERM, Duration::from_millis(200))
        .await
        .unwrap();
    assert!(!status.success(), "SIGKILL ended it");
}

#[tokio::test]
async fn a_child_that_already_exited_is_left_alone() {
    let mut child = shell("exit 0");
    child.wait().await.unwrap();
    send(&child, Signal::TERM);
}
