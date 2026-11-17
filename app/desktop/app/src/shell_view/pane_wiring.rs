use std::rc::Rc;

use shell_data::ShellData;

use crate::daemon_link::DaemonLink;
use crate::shell_request::ShellRequest;
use crate::shell_state::ShellState;

pub(super) struct PaneWiring<'a> {
    pub(super) state: &'a ShellState,
    pub(super) link: &'a DaemonLink,
    pub(super) data: &'a Rc<ShellData>,
    pub(super) requests: &'a async_channel::Sender<ShellRequest>,
}
