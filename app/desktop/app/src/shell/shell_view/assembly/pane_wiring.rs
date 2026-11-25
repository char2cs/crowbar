use std::rc::Rc;

use crate::daemon::daemon_link::DaemonLink;
use crate::shell::inbox::shell_request::ShellRequest;
use crate::shell::shell_state::ShellState;

pub(crate) struct PaneWiring<'a> {
    pub(crate) state: &'a ShellState,
    pub(crate) link: &'a DaemonLink,
    pub(crate) data: &'a Rc<store::Store>,
    pub(crate) requests: &'a async_channel::Sender<ShellRequest>,
    pub(crate) drafts: &'a std::path::Path,
}
