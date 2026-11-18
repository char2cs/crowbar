//! A sink that never drains, for the bridge tests.

use std::pin::Pin;
use std::task::{Context, Poll};

use futures_util::Sink;
use tokio_tungstenite::tungstenite::{Error as WsError, Message};

#[derive(Debug, Default)]
pub(crate) struct StuckSink {
    pub(crate) polls: usize,
}

impl Sink<Message> for StuckSink {
    type Error = WsError;

    fn poll_ready(
        mut self: Pin<&mut Self>,
        _: &mut Context<'_>,
    ) -> Poll<Result<(), WsError>> {
        self.polls += 1;
        Poll::Pending
    }

    fn start_send(
        self: Pin<&mut Self>,
        _: Message,
    ) -> Result<(), WsError> {
        Ok(())
    }

    fn poll_flush(
        self: Pin<&mut Self>,
        _: &mut Context<'_>,
    ) -> Poll<Result<(), WsError>> {
        Poll::Pending
    }

    fn poll_close(
        self: Pin<&mut Self>,
        _: &mut Context<'_>,
    ) -> Poll<Result<(), WsError>> {
        Poll::Pending
    }
}
