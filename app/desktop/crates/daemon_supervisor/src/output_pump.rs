use std::sync::Arc;

use tokio::io::{AsyncBufReadExt, AsyncRead, BufReader};
use tokio::process::Child;

use crate::daemon_log::DaemonLog;

fn pump<R>(stream: R, prefix: &'static str, log: Arc<DaemonLog>)
where
    R: AsyncRead + Unpin + Send + 'static,
{
    tokio::spawn(async move {
        let mut lines = BufReader::new(stream).lines();
        while let Ok(Some(line)) = lines.next_line().await {
            log.line(prefix, &line);
        }
    });
}

pub(crate) fn capture(child: &mut Child, log: &Arc<DaemonLog>) {
    if let Some(out) = child.stdout.take() {
        pump(out, "[out]", Arc::clone(log));
    }
    if let Some(err) = child.stderr.take() {
        pump(err, "[err]", Arc::clone(log));
    }
}

#[cfg(test)]
#[path = "output_pump_tests.rs"]
mod tests;
