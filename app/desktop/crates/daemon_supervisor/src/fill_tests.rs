use std::io::ErrorKind;
use std::pin::Pin;
use std::task::{Context, Poll};

use tokio::io::ReadBuf;

use super::*;

struct Broken;

impl AsyncRead for Broken {
    fn poll_read(
        self: Pin<&mut Self>,
        _: &mut Context<'_>,
        _: &mut ReadBuf<'_>,
    ) -> Poll<std::io::Result<()>> {
        Poll::Ready(Err(std::io::Error::from(ErrorKind::Other)))
    }
}

#[tokio::test]
async fn data_resets_the_failure_count_and_reports_its_length() {
    let mut reader = BufReader::new(&b"abc"[..]);
    let mut failures = 3;
    assert!(matches!(
        fill_buffer(&mut reader, "[out]", &mut failures).await,
        Fill::Data(3)
    ));
    assert_eq!(failures, 0);
}

#[tokio::test]
async fn errors_are_retried_until_the_limit_and_then_given_up_on() {
    let mut reader = BufReader::new(Broken);
    let mut failures = 0;
    for _ in 1..MAX_CONSECUTIVE_READ_ERRORS {
        assert!(matches!(
            fill_buffer(&mut reader, "[err]", &mut failures).await,
            Fill::Retry
        ));
    }
    assert!(matches!(
        fill_buffer(&mut reader, "[err]", &mut failures).await,
        Fill::GiveUp
    ));
}
