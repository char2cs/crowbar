use std::rc::Rc;

use crate::daemon_link::DaemonLink;
use crate::shell_request::ShellRequest;
use crate::shell_state::ShellState;

pub(super) struct PaneWiring<'a> {
    pub(super) state: &'a ShellState,
    pub(super) link: &'a DaemonLink,
    pub(super) data: &'a Rc<store::Store>,
    pub(super) requests: &'a async_channel::Sender<ShellRequest>,
    pub(super) drafts: &'a std::path::Path,
}
