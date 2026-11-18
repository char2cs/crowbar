use std::collections::VecDeque;
use std::future::Future;
use std::sync::Mutex;

use crate::error::Error;
use crate::link::PeerConnector;
use crate::test_support::FakeStream;

pub(crate) struct FakeConnector {
    lines: Mutex<VecDeque<Result<FakeStream, Error>>>,
}

impl FakeConnector {
    pub(crate) fn offering(lines: Vec<Result<FakeStream, Error>>) -> Self {
        Self {
            lines: Mutex::new(lines.into()),
        }
    }
}

impl PeerConnector for FakeConnector {
    type Stream = FakeStream;

    fn connect(&self) -> impl Future<Output = Result<FakeStream, Error>> + Send {
        let next = self
            .lines
            .lock()
            .expect("the line list")
            .pop_front()
            .unwrap_or_else(|| Err(Error::Stream("no daemon".into())));
        std::future::ready(next)
    }
}
