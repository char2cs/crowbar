use tokio::io::{AsyncBufReadExt, AsyncRead, BufReader};

const MAX_CONSECUTIVE_READ_ERRORS: u32 = 8;

pub(crate) enum Fill {
    Data(usize),
    Retry,
    GiveUp,
}

pub(crate) async fn fill_buffer<R: AsyncRead + Unpin>(
    reader: &mut BufReader<R>,
    prefix: &str,
    failures: &mut u32,
) -> Fill {
    match reader.fill_buf().await {
        Ok(chunk) => {
            *failures = 0;
            Fill::Data(chunk.len())
        },
        Err(e) => {
            *failures += 1;
            tracing::warn!("daemon {prefix} stream: {e}");
            if *failures >= MAX_CONSECUTIVE_READ_ERRORS {
                Fill::GiveUp
            } else {
                Fill::Retry
            }
        },
    }
}

#[cfg(test)]
#[path = "fill_tests.rs"]
mod tests;
