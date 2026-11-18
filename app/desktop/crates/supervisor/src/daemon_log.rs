use std::path::Path;
use std::sync::Arc;
use std::sync::atomic::{AtomicU64, Ordering};

use tokio::sync::{mpsc, oneshot};

use crate::adapters::{LogWriter, private_dir};
use crate::error::Error;
use crate::log_message::LogMessage;
use crate::log_sink::LogSink;
use crate::reply::reply;

const QUEUE_LINES: usize = 512;

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
    (0..=text.len().min(limit))
        .rev()
        .find(|&cut| text.is_char_boundary(cut))
        .unwrap_or(0)
}

fn pieces(line: &str) -> impl Iterator<Item = &str> {
    let mut remaining = Some(line);
    std::iter::from_fn(move || {
        let current = remaining.take()?;
        let (head, tail) = current.split_at(cut_at(current, MAX_LINE_BYTES));
        remaining = (!tail.is_empty()).then_some(tail);
        Some(head)
    })
}

fn format_lines(
    prefix: &str,
    text: &str,
) -> String {
    text.lines()
        .flat_map(|line| pieces(line.trim_end_matches('\r')))
        .fold(String::new(), |mut formatted, piece| {
            formatted.push_str(prefix);
            formatted.push(' ');
            formatted.push_str(piece);
            formatted.push('\n');
            formatted
        })
}

fn note_drops(
    writer: &mut dyn LogSink,
    lost: u64,
) -> std::io::Result<()> {
    if lost == 0 {
        return Ok(());
    }
    writer.append(&format!(
        "[log] {lost} log entries were dropped: the disk is slow\n"
    ))
}

fn write_message(
    writer: &mut dyn LogSink,
    message: LogMessage,
) -> std::io::Result<()> {
    match message {
        LogMessage::Text(text) => writer.append(&text),
        LogMessage::Flush(done) => {
            reply(done, ());
            Ok(())
        },
    }
}

fn run_writer(
    mut writer: Box<dyn LogSink>,
    mut queue: mpsc::Receiver<LogMessage>,
    dropped: &AtomicU64,
    path: &Path,
) {
    while let Some(message) = queue.blocking_recv() {
        write_next(writer.as_mut(), message, dropped, path);
    }
}

fn write_next(
    writer: &mut dyn LogSink,
    message: LogMessage,
    dropped: &AtomicU64,
    path: &Path,
) {
    let written = note_drops(writer, dropped.swap(0, Ordering::Relaxed))
        .and_then(|()| write_message(writer, message));
    if let Err(e) = written {
        tracing::warn!("daemon log {}: {e}", path.display());
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
            private_dir(dir).map_err(log_error)?;
        }
        let writer = LogWriter::open(path, max_bytes).map_err(log_error)?;
        Self::over(Box::new(writer), path).map_err(log_error)
    }

    pub(crate) fn over(
        sink: Box<dyn LogSink>,
        label: &Path,
    ) -> std::io::Result<Self> {
        let (queue, receiver) = mpsc::channel(QUEUE_LINES);
        let dropped = Arc::new(AtomicU64::new(0));
        let (own, owned_label) = (Arc::clone(&dropped), label.to_path_buf());
        std::thread::Builder::new()
            .name("daemon-log".to_owned())
            .spawn(move || run_writer(sink, receiver, &own, &owned_label))?;
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
    use crate::test_support::memory_sink::MemorySink;

    fn memory_log() -> (DaemonLog, MemorySink) {
        let sink = MemorySink::default();
        let log = DaemonLog::over(Box::new(sink.clone()), Path::new("memory")).unwrap();
        (log, sink)
    }

    async fn written(
        sink: &MemorySink,
        log: &DaemonLog,
    ) -> String {
        log.flush().await;
        sink.text()
    }

    fn log_all(
        log: &DaemonLog,
        lines: Lines<'_>,
    ) {
        for (prefix, text) in lines {
            log.line(prefix, text);
        }
    }

    type Lines<'a> = Vec<(&'a str, &'a str)>;

    async fn assert_lines_written(cases: Vec<(&str, Lines<'_>, &str)>) {
        for (name, lines, expected) in cases {
            let (log, sink) = memory_log();
            log_all(&log, lines);
            assert_eq!(written(&sink, &log).await, expected, "{name}");
        }
    }

    #[tokio::test]
    async fn lines_are_written_prefixed_one_per_line_without_doubling_newlines() {
        let cases = vec![
            (
                "carriage returns and consecutive lines",
                vec![("[out]", "hello\r\n"), ("===", "daemon spawned")],
                "[out] hello\n=== daemon spawned\n",
            ),
            (
                "several lines in one text",
                vec![("=== dump", "goroutine 1\n\ngoroutine 2\r\n")],
                "=== dump goroutine 1\n=== dump \n=== dump goroutine 2\n",
            ),
        ];
        assert_lines_written(cases).await;
    }

    #[tokio::test]
    #[ignore = "real-infra: asserts the 0600 and 0700 modes of a real log file and directory"]
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
    async fn a_line_longer_than_the_cap_is_written_in_pieces_without_splitting_a_character() {
        let (log, sink) = memory_log();
        let long = "é".repeat(MAX_LINE_BYTES);
        log.line("[out]", &long);
        let text = written(&sink, &log).await;
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
        let (log, sink) = memory_log();
        log.dropped.store(5, Ordering::Relaxed);
        log.line("[out]", "after");
        let text = written(&sink, &log).await;
        assert!(text.contains("[log] 5 log entries were dropped"), "{text}");
        assert!(text.ends_with("[out] after\n"), "{text}");
    }
}
