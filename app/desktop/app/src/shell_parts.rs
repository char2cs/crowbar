use std::path::PathBuf;
use std::rc::Rc;

use crate::daemon_link::DaemonLink;
use crate::shell_state::ShellState;

pub(crate) struct ShellParts {
    pub(crate) state: ShellState,
    pub(crate) link: Rc<DaemonLink>,
    pub(crate) preferences_path: PathBuf,
}
