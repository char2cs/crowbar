use tokio::process::Child;
use tokio::sync::mpsc;

use crate::watchdog_guard::WatchdogGuard;

pub(crate) struct Running {
    pub(crate) _watchdog: WatchdogGuard,
    pub(crate) child: Child,
    pub(crate) wedged: mpsc::Receiver<()>,
}
