use std::sync::mpsc;
use std::time::Duration;

use crate::error::{Error, Result};

pub(crate) fn wait_for<T>(
    answer: &mpsc::Receiver<Result<T>>,
    patience: Duration,
) -> Result<T> {
    match answer.recv_timeout(patience) {
        Ok(result) => result,
        Err(mpsc::RecvTimeoutError::Timeout) => Err(Error::TimedOut),
        Err(mpsc::RecvTimeoutError::Disconnected) => Err(Error::HostStopped),
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    const SHORT: Duration = Duration::from_millis(30);

    #[test]
    fn a_reply_that_never_comes_is_a_timeout_not_a_hang() {
        let (reply, answer) = mpsc::channel::<Result<u8>>();
        assert!(matches!(wait_for(&answer, SHORT), Err(Error::TimedOut)));
        reply.send(Ok(1)).unwrap();
        assert_eq!(
            wait_for(&answer, SHORT).unwrap(),
            1,
            "a late reply still counts"
        );
        drop(reply);
        assert!(matches!(wait_for(&answer, SHORT), Err(Error::HostStopped)));
    }
}
