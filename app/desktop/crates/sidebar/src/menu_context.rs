//! What a row menu needs to know beyond the row.

use crate::move_target::MoveTarget;
use crate::terminal_threads::TerminalThreads;

#[derive(Debug, Clone, PartialEq, Eq, Default)]
pub(crate) struct MenuContext {
    pub(crate) targets: Vec<MoveTarget>,
    pub(crate) terminal: TerminalThreads,
    pub(crate) running: Option<String>,
}
