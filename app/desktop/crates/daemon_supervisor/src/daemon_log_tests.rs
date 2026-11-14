use super::*;

fn dir(tag: &str) -> PathBuf {
    let dir = std::env::temp_dir().join(format!("cb-dlog-{tag}-{}", std::process::id()));
    std::fs::create_dir_all(&dir).unwrap();
    dir
}

#[test]
fn lines_are_appended_with_their_prefix_and_no_trailing_newline_doubling() {
    let path = dir("append").join("logs").join("daemon.log");
    let log = DaemonLog::open(&path, 1 << 20).unwrap();
    log.line("[out]", "hello\r\n");
    log.line("===", "daemon spawned");
    assert_eq!(
        std::fs::read_to_string(&path).unwrap(),
        "[out] hello\n=== daemon spawned\n"
    );
}

#[test]
fn a_full_log_is_rotated_aside_when_the_next_run_opens_it() {
    let dir = dir("rotate");
    let path = dir.join("daemon.log");
    std::fs::write(&path, b"0123456789ABCDEF").unwrap();
    DaemonLog::open(&path, 10).unwrap().line("[out]", "fresh");
    assert_eq!(
        std::fs::read(dir.join("daemon.log.1")).unwrap(),
        b"0123456789ABCDEF"
    );
    assert_eq!(std::fs::read_to_string(&path).unwrap(), "[out] fresh\n");
}

#[test]
fn a_log_below_the_cap_is_left_in_place() {
    let dir = dir("below");
    let path = dir.join("daemon.log");
    std::fs::write(&path, b"short").unwrap();
    DaemonLog::open(&path, 10).unwrap();
    assert!(!dir.join("daemon.log.1").exists());
}
