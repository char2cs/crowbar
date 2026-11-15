use std::os::unix::fs::DirBuilderExt;
use std::path::Path;
use std::sync::Arc;
use std::sync::atomic::{AtomicU64, Ordering};

use tokio::sync::{mpsc, oneshot};

use crate::error::Error;
use crate::log_message::LogMessage;
use crate::log_writer::LogWriter;
use crate::reply::reply;

const QUEUE_LINES: usize = 512;
const DIR_MODE: u32 = 0o700;

pub(crate) const MAX_LINE_BYTES: usize = 16 * 1024;

/// The daemon's captured stdout and stderr, one line per entry, rotated to `.1` at a size cap.
/// It is the only place a Go panic's stack trace survives in the packaged app.
///
/// Writing never blocks the caller: text goes through a bounded queue to one writer thread, and
/// when the disk cannot keep up the newest text is dropped and counted, never the process
/// stalled. The file is private to the user (`0600` in a `0700` directory) because it can hold
/// prompts, paths and stack traces.
#[derive(Debug)]
pub struct DaemonLog {
    queue: mpsc::Sender<LogMessage>,
    dropped: Arc<AtomicU64>,
}

fn cut_at(
    text: &str,
    limit: usize,
) -> usize {
    let mut cut = text.len().min(limit);
    while !text.is_char_boundary(cut) {
        cut -= 1;
    }
    cut
}

fn push_line(
    out: &mut String,
    prefix: &str,
    mut line: &str,
) {
    loop {
        let (head, tail) = line.split_at(cut_at(line, MAX_LINE_BYTES));
        out.push_str(prefix);
        out.push(' ');
        out.push_str(head);
        out.push('\n');
        if tail.is_empty() {
            return;
        }
        line = tail;
    }
}

fn format_lines(
    prefix: &str,
    text: &str,
) -> String {
    let mut out = String::new();
    for line in text.lines() {
        push_line(&mut out, prefix, line.trim_end_matches('\r'));
    }
    out
}

fn run_writer(
    mut writer: LogWriter,
    mut queue: mpsc::Receiver<LogMessage>,
    dropped: &AtomicU64,
    path: &Path,
) {
    while let Some(message) = queue.blocking_recv() {
        let lost = dropped.swap(0, Ordering::Relaxed);
        let result = if lost > 0 {
            writer.write(&format!(
                "[log] {lost} log entries were dropped: the disk is slow\n"
            ))
        } else {
            Ok(())
        };
        let result = result.and_then(|()| match message {
            LogMessage::Text(text) => writer.write(&text),
            LogMessage::Flush(done) => {
                reply(done, ());
                Ok(())
            },
        });
        if let Err(e) = result {
            tracing::warn!("daemon log {}: {e}", path.display());
        }
    }
}

impl DaemonLog {
    /// Opens `path` for appending, creating its directory and rotating a full previous log.
    ///
    /// # Errors
    /// [`Error::Log`] when the file or the writer thread cannot be set up.
    pub fn open(
        path: &Path,
        max_bytes: u64,
    ) -> Result<Self, Error> {
        let log_error = |source| Error::Log {
            path: path.to_path_buf(),
            source,
        };
        if let Some(dir) = path.parent() {
            std::fs::DirBuilder::new()
                .recursive(true)
                .mode(DIR_MODE)
                .create(dir)
                .map_err(log_error)?;
        }
        let writer = LogWriter::open(path, max_bytes).map_err(log_error)?;
        let (queue, receiver) = mpsc::channel(QUEUE_LINES);
        let dropped = Arc::new(AtomicU64::new(0));
        let (own, owned_path) = (Arc::clone(&dropped), path.to_path_buf());
        std::thread::Builder::new()
            .name("daemon-log".to_owned())
            .spawn(move || run_writer(writer, receiver, &own, &owned_path))
            .map_err(log_error)?;
        Ok(Self { queue, dropped })
    }

    /// Queues `prefix text` for writing. Text with several lines becomes several prefixed
    /// lines, and a line longer than the cap is cut into pieces. When the queue is full the
    /// text is dropped, and a later line says how much.
    pub fn line(
        &self,
        prefix: &str,
        text: &str,
    ) {
        let formatted = format_lines(prefix, text);
        if self.queue.try_send(LogMessage::Text(formatted)).is_err() {
            self.dropped.fetch_add(1, Ordering::Relaxed);
        }
    }

    /// Resolves once everything queued so far is on disk.
    pub async fn flush(&self) {
        let (done, written) = oneshot::channel();
        if self.queue.send(LogMessage::Flush(done)).await.is_ok() && written.await.is_err() {
            tracing::debug!("the daemon log writer ended before a flush finished");
        }
    }
}

#[cfg(test)]
mod tests {
    use std::os::unix::fs::PermissionsExt;

    use super::*;

    async fn written(
        path: &Path,
        log: &DaemonLog,
    ) -> String {
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
}
