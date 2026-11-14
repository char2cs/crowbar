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
