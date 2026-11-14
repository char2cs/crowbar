use std::path::PathBuf;
use std::sync::Arc;

use tokio::sync::mpsc;
use tokio_util::sync::CancellationToken;

use crate::bus::Bus;
use crate::timings::Timings;

pub(crate) struct WatchContext {
    pub(crate) socket: PathBuf,
    pub(crate) pid: Option<u32>,
    pub(crate) timings: Timings,
    pub(crate) bus: Arc<Bus>,
    pub(crate) wedged: mpsc::Sender<()>,
    pub(crate) cancel: CancellationToken,
}
