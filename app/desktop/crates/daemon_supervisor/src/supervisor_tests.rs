use std::path::PathBuf;

use super::*;
use crate::daemon_paths::DaemonPaths;

fn missing_binary(tag: &str) -> SupervisorConfig {
    let dir = std::env::temp_dir().join(format!("cb-supv-{tag}-{}", std::process::id()));
    let paths = DaemonPaths {
        socket: dir.join("d.sock"),
        log: dir.join("d.log"),
    };
    let mut config =
        SupervisorConfig::new(PathBuf::from("/definitely/not/crowbar-api"), paths, None);
    config.timings.respawn_base = std::time::Duration::from_millis(1);
    config
}

#[tokio::test]
async fn a_restart_while_the_daemon_is_not_up_is_accepted_and_one_after_stop_is_refused() {
    let supervisor = Supervisor::start(missing_binary("restart"));
    let mut events = supervisor.events();
    while !matches!(events.recv().await, Ok(DaemonEvent::Recovering { .. })) {}
    supervisor.restart().await.unwrap();
    let commands = supervisor.commands.clone();
    supervisor.stop().await;
    let (reply, _answer) = oneshot::channel();
    assert!(
        commands.send(Command::Restart(reply)).await.is_err(),
        "supervision has ended"
    );
}
