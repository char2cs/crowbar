use tokio::process::Child;
use tokio::sync::mpsc;
use tokio_util::sync::CancellationToken;

pub(crate) struct Running {
    pub(crate) child: Child,
    pub(crate) wedged: mpsc::Receiver<()>,
    pub(crate) watchdog: CancellationToken,
}

impl Drop for Running {
    fn drop(&mut self) {
        self.watchdog.cancel();
    }
}

#[cfg(test)]
mod tests {
    use std::process::Stdio;

    use tokio::process::Command;

    use super::*;

    #[tokio::test]
    async fn dropping_a_running_daemon_stops_its_watchdog() {
        let child = Command::new("/bin/sh")
            .args(["-c", "exit 0"])
            .stdin(Stdio::null())
            .spawn()
            .unwrap();
        let (_wedged, wedged) = mpsc::channel(1);
        let watchdog = CancellationToken::new();
        let running = Running {
            child,
            wedged,
            watchdog: watchdog.clone(),
        };
        assert!(!watchdog.is_cancelled());
        drop(running);
        assert!(watchdog.is_cancelled());
    }
}
