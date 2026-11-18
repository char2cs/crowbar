use tokio::process::Child;
use tokio::sync::mpsc;

use crate::supervisor::monitoring::watchdog::WatchdogGuard;

pub(crate) struct Running {
    pub(crate) _watchdog: WatchdogGuard,
    pub(crate) child: Child,
    pub(crate) wedged: mpsc::Receiver<()>,
}
