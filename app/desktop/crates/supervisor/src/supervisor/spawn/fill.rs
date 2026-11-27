use tokio::io::{AsyncBufReadExt, AsyncRead, BufReader};

const MAX_CONSECUTIVE_READ_ERRORS: u32 = 8;

pub(crate) enum Fill {
    Data(usize),
    Retry,
    GiveUp,
}

fn after_error(failures: u32) -> Fill {
    if failures >= MAX_CONSECUTIVE_READ_ERRORS {
        return Fill::GiveUp;
    }
    Fill::Retry
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
            after_error(*failures)
        },
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[tokio::test]
    async fn data_resets_the_failure_count_and_reports_its_length() {
        let mut reader = BufReader::new(&b"abc"[..]);
        let mut failures = 3;
        let filled = fill_buffer(&mut reader, "[out]", &mut failures).await;
        assert!(matches!(filled, Fill::Data(3)));
        assert_eq!(failures, 0);
    }
}
