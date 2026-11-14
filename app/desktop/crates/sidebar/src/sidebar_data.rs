//! What the sidebar reads from the shell.

use crate::changes::Changes;
use crate::file_row::FileRow;
use crate::focus::Focus;
use crate::host::Host;
use crate::move_target::MoveTarget;
use crate::project::Project;
use crate::project_id::ProjectId;
use crate::project_tree::ProjectTree;
use crate::row_id::RowId;
use crate::scope_sync::ScopeSync;

/// The read side of the sidebar, implemented by the shell over the daemon's entity streams. The
/// view calls it on every render and never keeps a copy of what it returns.
pub trait SidebarData {
    /// Whether the projects scope has synced.
    fn sync(&self) -> ScopeSync;
    /// Every known host.
    fn hosts(&self) -> Vec<Host>;
    /// The spaces of the selected host, in switcher order.
    fn projects(&self) -> Vec<Project>;
    /// The space shown, if any.
    fn active_project(&self) -> Option<ProjectId>;
    /// The chats tree of `project`.
    fn tree(&self, project: &ProjectId) -> ProjectTree;
    /// What the focused pane shows, which decides the available modes.
    fn focus(&self) -> Focus;
    /// The folders and branches `row` can move into.
    fn move_targets(&self, row: &RowId) -> Vec<MoveTarget>;
    /// Whether a thread can start in a terminal.
    fn can_start_terminal_thread(&self) -> bool;
    /// The visible rows of the Files panel.
    fn files(&self) -> Vec<FileRow>;
    /// What the Changes panel draws.
    fn changes(&self) -> Changes;
}

#[cfg(test)]
#[path = "sidebar_data_tests.rs"]
mod tests;
