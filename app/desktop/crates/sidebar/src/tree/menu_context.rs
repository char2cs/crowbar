//! What a row menu needs to know beyond the row.

use crate::shell::MoveTarget;
use crate::tree::terminal_threads::TerminalThreads;

/// The facts about the rest of the sidebar that decide a row menu's entries.
#[derive(Debug, Clone, PartialEq, Eq, Default)]
pub struct MenuContext {
    /// The legal "Move to…" destinations for the row.
    pub targets: Vec<MoveTarget>,
    /// Whether threads can start in a terminal.
    pub terminal: TerminalThreads,
    /// The title of the agent running on the row, if any; offers "Kill agent".
    pub running: Option<String>,
}
