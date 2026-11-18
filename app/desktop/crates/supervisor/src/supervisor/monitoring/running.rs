use tokio::process::Child;
use tokio::sync::mpsc;

use crate::supervisor::monitoring::watchdog::Guard;

pub(crate) struct Running {
    pub(crate) _watchdog: Guard,
    pub(crate) child: Child,
    pub(crate) wedged: mpsc::Receiver<()>,
}
