use tokio_util::sync::CancellationToken;

pub(crate) struct WatchdogGuard(CancellationToken);

impl WatchdogGuard {
    pub(crate) fn new(watchdog: CancellationToken) -> Self {
        Self(watchdog)
    }
}

impl Drop for WatchdogGuard {
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
        let guard = WatchdogGuard::new(watchdog.clone());
        assert!(!watchdog.is_cancelled());
        drop(guard);
        assert!(watchdog.is_cancelled());
    }
}
