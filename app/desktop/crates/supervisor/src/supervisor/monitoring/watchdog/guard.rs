use tokio_util::sync::CancellationToken;

pub(crate) struct Guard(CancellationToken);

impl Guard {
    pub(crate) fn new(watchdog: CancellationToken) -> Self {
        Self(watchdog)
    }
}

impl Drop for Guard {
    fn drop(&mut self) {
        self.0.cancel();
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn dropping_the_guard_cancels_the_watchdog() {
        let watchdog = CancellationToken::new();
        let guard = Guard::new(watchdog.clone());
        assert!(!watchdog.is_cancelled());
        drop(guard);
        assert!(watchdog.is_cancelled());
    }
}
