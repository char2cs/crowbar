use std::path::PathBuf;
use std::rc::Rc;
use std::sync::Arc;
use std::sync::atomic::AtomicBool;

use crate::daemon_link::DaemonLink;
use crate::ports::Ports;
use crate::shell_request::ShellRequest;
use crate::shell_state::ShellState;

pub(crate) struct ShellParts {
    pub(crate) state: ShellState,
    pub(crate) link: Rc<DaemonLink>,
    pub(crate) preferences_path: PathBuf,
    pub(crate) ports: Ports,
    pub(crate) data: Rc<store::Store>,
    pub(crate) data_queued: Arc<AtomicBool>,
    pub(crate) requests: async_channel::Sender<ShellRequest>,
    pub(crate) queue: async_channel::Receiver<ShellRequest>,
}
