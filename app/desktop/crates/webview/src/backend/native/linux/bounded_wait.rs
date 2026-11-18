use std::sync::mpsc;
use std::time::{Duration, Instant};

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

pub(crate) fn collect_within<T>(
    answers: &[mpsc::Receiver<T>],
    patience: Duration,
    now: impl Fn() -> Instant,
) -> Vec<Option<T>> {
    let deadline = now() + patience;
    answers
        .iter()
        .map(|answer| {
            answer
                .recv_timeout(deadline.saturating_duration_since(now()))
                .ok()
        })
        .collect()
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

    #[test]
    fn one_deadline_covers_every_answer() {
        let (fast, fast_answer) = mpsc::channel();
        let (_silent, silent_answer) = mpsc::channel::<u8>();
        let (_late, late_answer) = mpsc::channel::<u8>();
        fast.send(7_u8).unwrap();
        let started = Instant::now();
        let answers = collect_within(
            &[fast_answer, silent_answer, late_answer],
            SHORT * 2,
            Instant::now,
        );
        assert_eq!(answers, [Some(7), None, None], "only the fast one answered");
        assert!(
            started.elapsed() < SHORT * 4,
            "two silent views did not each wait their own full patience"
        );
    }
}
