use std::path::PathBuf;

use super::*;

fn options(home: &Path) -> AppOptions {
    AppOptions {
        panes_dir: PathBuf::from("panes"),
        chat_entry: "chat/index.html".into(),
        home: home.to_path_buf(),
        daemon_binary: Some(home.join("missing-daemon")),
    }
}

#[test]
fn a_missing_daemon_binary_is_reported_as_a_halt() {
    let home = tempfile::tempdir().unwrap();
    let link = DaemonLink::start(&options(home.path())).unwrap();
    let phase = link.phases().try_recv().unwrap();
    assert!(
        matches!(phase, StartupInput::Halted { .. }),
        "the startup view explains it: {phase:?}"
    );
    assert!(link.log().is_none(), "nothing to open");
    link.restart();
}
