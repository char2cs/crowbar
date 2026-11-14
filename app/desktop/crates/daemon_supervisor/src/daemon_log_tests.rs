use std::os::unix::fs::PermissionsExt;

use super::*;

async fn written(path: &Path, log: &DaemonLog) -> String {
    log.flush().await;
    std::fs::read_to_string(path).unwrap()
}

#[tokio::test]
async fn lines_are_appended_with_their_prefix_and_no_trailing_newline_doubling() {
    let dir = tempfile::tempdir().unwrap();
    let path = dir.path().join("logs").join("daemon.log");
    let log = DaemonLog::open(&path, 1 << 20).unwrap();
    log.line("[out]", "hello\r\n");
    log.line("===", "daemon spawned");
    assert_eq!(
        written(&path, &log).await,
        "[out] hello\n=== daemon spawned\n"
    );
}

#[tokio::test]
async fn the_log_directory_and_file_are_private() {
    let dir = tempfile::tempdir().unwrap();
    let path = dir.path().join("logs").join("daemon.log");
    let log = DaemonLog::open(&path, 1 << 20).unwrap();
    log.flush().await;
    let mode = |p: &Path| std::fs::metadata(p).unwrap().permissions().mode() & 0o777;
    assert_eq!(mode(path.parent().unwrap()), 0o700, "directory");
    assert_eq!(mode(&path), 0o600, "file");
}

#[tokio::test]
async fn text_with_several_lines_becomes_several_prefixed_lines() {
    let dir = tempfile::tempdir().unwrap();
    let path = dir.path().join("daemon.log");
    let log = DaemonLog::open(&path, 1 << 20).unwrap();
    log.line("=== dump", "goroutine 1\n\ngoroutine 2\r\n");
    assert_eq!(
        written(&path, &log).await,
        "=== dump goroutine 1\n=== dump \n=== dump goroutine 2\n"
    );
}

#[tokio::test]
async fn a_line_longer_than_the_cap_is_written_in_pieces_without_splitting_a_character() {
    let dir = tempfile::tempdir().unwrap();
    let path = dir.path().join("daemon.log");
    let log = DaemonLog::open(&path, 1 << 20).unwrap();
    let long = "é".repeat(MAX_LINE_BYTES);
    log.line("[out]", &long);
    let text = written(&path, &log).await;
    let lines: Vec<&str> = text.lines().collect();
    assert_eq!(lines.len(), 2, "cut once: {} bytes", long.len());
    assert!(
        lines
            .iter()
            .all(|line| line.len() <= MAX_LINE_BYTES + "[out] ".len()),
        "each within the cap"
    );
    let joined: String = lines
        .iter()
        .filter_map(|line| line.strip_prefix("[out] "))
        .collect();
    assert_eq!(joined, long, "nothing lost");
}

#[tokio::test]
async fn the_log_rotates_by_size_while_the_daemon_runs() {
    let dir = tempfile::tempdir().unwrap();
    let path = dir.path().join("daemon.log");
    let log = DaemonLog::open(&path, 40).unwrap();
    for n in 0..6 {
        log.line("[out]", &format!("line number {n}"));
    }
    let text = written(&path, &log).await;
    assert!(dir.path().join("daemon.log.1").exists(), "rotated");
    assert!(
        text.len() <= 40 + "[out] line number 5\n".len(),
        "current file stays near the cap: {text:?}"
    );
}

#[test]
fn a_full_queue_drops_new_text_and_counts_it_instead_of_blocking() {
    let (queue, _unread) = mpsc::channel(1);
    let log = DaemonLog {
        queue,
        dropped: Arc::new(AtomicU64::new(0)),
    };
    for _ in 0..4 {
        log.line("[out]", "x");
    }
    assert_eq!(log.dropped.load(Ordering::Relaxed), 3);
}

#[tokio::test]
async fn the_writer_says_how_many_entries_it_had_to_drop() {
    let dir = tempfile::tempdir().unwrap();
    let path = dir.path().join("daemon.log");
    let log = DaemonLog::open(&path, 1 << 20).unwrap();
    log.dropped.store(5, Ordering::Relaxed);
    log.line("[out]", "after");
    let text = written(&path, &log).await;
    assert!(text.contains("[log] 5 log entries were dropped"), "{text}");
    assert!(text.ends_with("[out] after\n"), "{text}");
}
