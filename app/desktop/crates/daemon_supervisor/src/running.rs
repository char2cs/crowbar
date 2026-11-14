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
#[path = "running_tests.rs"]
mod tests;
