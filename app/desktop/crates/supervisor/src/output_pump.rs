use std::sync::Arc;

use tokio::io::{AsyncBufReadExt, AsyncRead, BufReader};
use tokio::process::Child;

use crate::daemon_log::{DaemonLog, MAX_LINE_BYTES};
use crate::fill::{Fill, fill_buffer};

fn cut_full_pieces(
    pending: &mut Vec<u8>,
    emit: &mut impl FnMut(&[u8]),
) {
    while pending.len() >= MAX_LINE_BYTES {
        let piece: Vec<u8> = pending.drain(..MAX_LINE_BYTES).collect();
        emit(&piece);
    }
}

pub(crate) fn split_lines(
    chunk: &[u8],
    pending: &mut Vec<u8>,
    mut emit: impl FnMut(&[u8]),
) {
    for segment in chunk.split_inclusive(|byte| *byte == b'\n') {
        take_segment(segment, pending, &mut emit);
    }
}

fn take_segment(
    segment: &[u8],
    pending: &mut Vec<u8>,
    emit: &mut impl FnMut(&[u8]),
) {
    let (body, complete) = match segment.split_last() {
        Some((b'\n', body)) => (body, true),
        _ => (segment, false),
    };
    pending.extend_from_slice(body);
    cut_full_pieces(pending, emit);
    if complete {
        emit(pending);
        pending.clear();
    }
}

struct OutputPump<R> {
    reader: BufReader<R>,
    pending: Vec<u8>,
    failures: u32,
    prefix: &'static str,
    log: Arc<DaemonLog>,
}

impl<R: AsyncRead + Unpin> OutputPump<R> {
    fn new(
        stream: R,
        prefix: &'static str,
        log: Arc<DaemonLog>,
    ) -> Self {
        Self {
            reader: BufReader::new(stream),
            pending: Vec::new(),
            failures: 0,
            prefix,
            log,
        }
    }

    async fn step(&mut self) -> bool {
        let filled = fill_buffer(&mut self.reader, self.prefix, &mut self.failures).await;
        let available = match filled {
            Fill::Data(len) => len,
            Fill::Retry => return true,
            Fill::GiveUp => return false,
        };
        if available == 0 {
            return false;
        }
        let (prefix, log) = (self.prefix, &self.log);
        split_lines(self.reader.buffer(), &mut self.pending, |line| {
            log.line(prefix, &String::from_utf8_lossy(line));
        });
        self.reader.consume(available);
        true
    }

    fn finish(self) {
        if !self.pending.is_empty() {
            self.log
                .line(self.prefix, &String::from_utf8_lossy(&self.pending));
        }
    }
}

async fn pump<R>(
    stream: R,
    prefix: &'static str,
    log: Arc<DaemonLog>,
) where
    R: AsyncRead + Unpin,
{
    let mut lines = OutputPump::new(stream, prefix, log);
    while lines.step().await {}
    lines.finish();
}

pub(crate) fn capture(
    child: &mut Child,
    log: &Arc<DaemonLog>,
) {
    if let Some(out) = child.stdout.take() {
        tokio::spawn(pump(out, "[out]", Arc::clone(log)));
    }
    if let Some(err) = child.stderr.take() {
        tokio::spawn(pump(err, "[err]", Arc::clone(log)));
    }
}

#[cfg(test)]
mod tests {
    use std::io::ErrorKind;
    use std::path::Path;
    use std::pin::Pin;
    use std::task::{Context, Poll};

    use tokio::io::{AsyncWriteExt, ReadBuf};

    use super::*;
    use crate::test_support::memory_sink::MemorySink;

    fn lines_of(chunks: &[&[u8]]) -> Vec<Vec<u8>> {
        let (mut pending, mut out) = (Vec::new(), Vec::new());
        for chunk in chunks {
            split_lines(chunk, &mut pending, |line| out.push(line.to_vec()));
        }
        out
    }

    fn memory_log() -> (Arc<DaemonLog>, MemorySink) {
        let sink = MemorySink::default();
        let log = DaemonLog::over(Box::new(sink.clone()), Path::new("memory")).unwrap();
        (Arc::new(log), sink)
    }

    async fn pumped(input: &[u8]) -> String {
        let (log, sink) = memory_log();
        let (mut writer, reader) = tokio::io::duplex(1024);
        let task = tokio::spawn(pump(reader, "[out]", Arc::clone(&log)));
        writer.write_all(input).await.unwrap();
        drop(writer);
        task.await.unwrap();
        log.flush().await;
        sink.text()
    }

    /// Fails `failures` times, then yields `data` and ends.
    struct Flaky {
        failures: u32,
        data: Option<&'static [u8]>,
    }

    impl AsyncRead for Flaky {
        fn poll_read(
            mut self: Pin<&mut Self>,
            _: &mut Context<'_>,
            buf: &mut ReadBuf<'_>,
        ) -> Poll<std::io::Result<()>> {
            let failing = self.failures > 0;
            self.failures = self.failures.saturating_sub(1);
            let data = (!failing).then(|| self.data.take()).flatten();
            buf.put_slice(data.unwrap_or_default());
            let failure = std::io::Error::from(ErrorKind::Other);
            Poll::Ready(failing.then_some(failure).map_or(Ok(()), Err))
        }
    }

    async fn pumped_from(reader: Flaky) -> String {
        let (log, sink) = memory_log();
        pump(reader, "[err]", Arc::clone(&log)).await;
        log.flush().await;
        sink.text()
    }

    #[test]
    fn lines_are_cut_at_newlines_across_chunks() {
        let out = lines_of(&[b"one\ntw", b"o\n\nthr", b"ee"]);
        assert_eq!(
            out,
            [b"one".to_vec(), b"two".to_vec(), Vec::new()],
            "unfinished tail is held back"
        );
    }

    #[test]
    fn a_line_that_never_ends_is_cut_into_pieces_of_the_cap() {
        let endless = vec![b'x'; MAX_LINE_BYTES * 2 + 5];
        let mut pending = Vec::new();
        let mut pieces = Vec::new();
        split_lines(&endless, &mut pending, |line| pieces.push(line.len()));
        assert_eq!(pieces, [MAX_LINE_BYTES, MAX_LINE_BYTES]);
        assert_eq!(pending.len(), 5, "memory stays bounded by the cap");
    }

    #[tokio::test]
    async fn output_that_is_not_utf8_is_logged_lossily_and_the_pump_keeps_reading() {
        let text = pumped(
            b"before\nbroken \xe2\x82 tail\n\xff\xfe\nafter the bad bytes\nno newline at the end",
        )
        .await;
        assert_eq!(
            text,
            "[out] before\n[out] broken \u{fffd} tail\n[out] \u{fffd}\u{fffd}\n[out] after the bad bytes\n[out] no newline at the end\n"
        );
    }

    async fn assert_each_pumped(cases: [(&str, Flaky, &str); 2]) {
        for (name, reader, expected) in cases {
            assert_eq!(pumped_from(reader).await, expected, "{name}");
        }
    }

    #[tokio::test]
    async fn read_errors_are_retried_until_the_stream_is_given_up_on() {
        let cases = [
            (
                "later output is kept",
                Flaky {
                    failures: 3,
                    data: Some(b"still here\n"),
                },
                "[err] still here\n",
            ),
            (
                "a stream that only fails is not spun on",
                Flaky {
                    failures: u32::MAX,
                    data: Some(b"never read\n"),
                },
                "",
            ),
        ];
        assert_each_pumped(cases).await;
    }
}
