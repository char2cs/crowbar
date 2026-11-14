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

fn cut_at(text: &str, limit: usize) -> usize {
    let mut cut = text.len().min(limit);
    while !text.is_char_boundary(cut) {
        cut -= 1;
    }
    cut
}

fn push_line(out: &mut String, prefix: &str, mut line: &str) {
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

fn format_lines(prefix: &str, text: &str) -> String {
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
    pub fn open(path: &Path, max_bytes: u64) -> Result<Self, Error> {
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
    pub fn line(&self, prefix: &str, text: &str) {
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
#[path = "daemon_log_tests.rs"]
mod tests;
