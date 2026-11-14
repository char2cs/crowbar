use std::sync::Arc;

use tokio::io::{AsyncBufReadExt, AsyncRead, BufReader};
use tokio::process::Child;

use crate::daemon_log::{DaemonLog, MAX_LINE_BYTES};
use crate::fill::{Fill, fill_buffer};

pub(crate) fn split_lines(chunk: &[u8], pending: &mut Vec<u8>, mut emit: impl FnMut(&[u8])) {
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

async fn pump<R>(stream: R, prefix: &'static str, log: Arc<DaemonLog>)
where
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

pub(crate) fn capture(child: &mut Child, log: &Arc<DaemonLog>) {
    if let Some(out) = child.stdout.take() {
        tokio::spawn(pump(out, "[out]", Arc::clone(log)));
    }
    if let Some(err) = child.stderr.take() {
        tokio::spawn(pump(err, "[err]", Arc::clone(log)));
    }
}

#[cfg(test)]
#[path = "output_pump_tests.rs"]
mod tests;
