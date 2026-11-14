//! What a row menu needs to know beyond the row.

use crate::move_target::MoveTarget;
use crate::terminal_threads::TerminalThreads;

/// Capabilities and destinations that depend on the rest of the shell.
#[derive(Debug, Clone, PartialEq, Eq, Default)]
pub struct MenuContext {
    /// Legal "Move to…" destinations.
    pub targets: Vec<MoveTarget>,
    /// The enabled provider can start a thread in a terminal.
    pub terminal: TerminalThreads,
}
