use std::sync::Arc;

use tokio::io::{AsyncBufReadExt, AsyncRead, BufReader};
use tokio::process::Child;

use crate::daemon_log::{DaemonLog, MAX_LINE_BYTES};
use crate::fill::{Fill, fill_buffer};

pub(crate) fn split_lines(
    chunk: &[u8],
    pending: &mut Vec<u8>,
    mut emit: impl FnMut(&[u8]),
) {
    for segment in chunk.split_inclusive(|byte| *byte == b'\n') {
        let (body, complete) = match segment.split_last() {
            Some((b'\n', body)) => (body, true),
            _ => (segment, false),
        };
        pending.extend_from_slice(body);
        while pending.len() >= MAX_LINE_BYTES {
            let piece: Vec<u8> = pending.drain(..MAX_LINE_BYTES).collect();
            emit(&piece);
        }
        if complete {
            emit(pending);
            pending.clear();
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
    let mut reader = BufReader::new(stream);
    let mut pending = Vec::new();
    let mut failures = 0;
    let mut emit = |line: &[u8]| log.line(prefix, &String::from_utf8_lossy(line));
    loop {
        let available = match fill_buffer(&mut reader, prefix, &mut failures).await {
            Fill::Data(len) => len,
            Fill::Retry => continue,
            Fill::GiveUp => break,
        };
        if available == 0 {
            break;
        }
        split_lines(reader.buffer(), &mut pending, &mut emit);
        reader.consume(available);
    }
    if !pending.is_empty() {
        emit(&pending);
    }
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
    use std::pin::Pin;
    use std::task::{Context, Poll};

    use tokio::io::{AsyncWriteExt, ReadBuf};

    use super::*;

    fn lines_of(chunks: &[&[u8]]) -> Vec<Vec<u8>> {
        let (mut pending, mut out) = (Vec::new(), Vec::new());
        for chunk in chunks {
            split_lines(chunk, &mut pending, |line| out.push(line.to_vec()));
        }
        out
    }

    async fn pumped(input: &[u8]) -> String {
        let dir = tempfile::tempdir().unwrap();
        let path = dir.path().join("daemon.log");
        let log = Arc::new(DaemonLog::open(&path, 1 << 20).unwrap());
        let (mut writer, reader) = tokio::io::duplex(1024);
        let task = tokio::spawn(pump(reader, "[out]", Arc::clone(&log)));
        writer.write_all(input).await.unwrap();
        drop(writer);
        task.await.unwrap();
        log.flush().await;
        std::fs::read_to_string(&path).unwrap()
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
            if self.failures > 0 {
                self.failures -= 1;
                return Poll::Ready(Err(std::io::Error::from(ErrorKind::Other)));
            }
            if let Some(data) = self.data.take() {
                buf.put_slice(data);
            }
            Poll::Ready(Ok(()))
        }
    }

    async fn pumped_from(reader: Flaky) -> String {
        let dir = tempfile::tempdir().unwrap();
        let path = dir.path().join("daemon.log");
        let log = Arc::new(DaemonLog::open(&path, 1 << 20).unwrap());
        pump(reader, "[err]", Arc::clone(&log)).await;
        log.flush().await;
        std::fs::read_to_string(&path).unwrap()
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

    #[tokio::test]
    async fn a_read_error_does_not_end_the_pump_and_later_output_is_kept() {
        let text = pumped_from(Flaky {
            failures: 3,
            data: Some(b"still here\n"),
        })
        .await;
        assert_eq!(text, "[err] still here\n");
    }

    #[tokio::test]
    async fn a_stream_that_only_fails_is_given_up_on_instead_of_spinning() {
        let text = pumped_from(Flaky {
            failures: u32::MAX,
            data: Some(b"never read\n"),
        })
        .await;
        assert_eq!(text, "");
    }
}
