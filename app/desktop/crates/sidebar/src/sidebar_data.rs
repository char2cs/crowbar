//! What the sidebar reads from the shell.

use crate::changes::Changes;
use crate::command::Command;
use crate::file_row::FileRow;
use crate::focus::Focus;
use crate::host::Host;
use crate::merge_box::MergeBox;
use crate::move_target::MoveTarget;
use crate::project::Project;
use crate::project_id::ProjectId;
use crate::project_tree::ProjectTree;
use crate::row_id::RowId;
use crate::running_agent::RunningAgent;
use crate::scope_sync::ScopeSync;
use crate::space_icon::SpaceIcon;

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
    fn tree(
        &self,
        project: &ProjectId,
    ) -> ProjectTree;
    /// What the focused pane shows, which decides the available modes.
    fn focus(&self) -> Focus;
    /// The folders and branches `row` can move into.
    fn move_targets(
        &self,
        row: &RowId,
    ) -> Vec<MoveTarget>;
    /// Whether a thread can start in a terminal.
    fn can_start_terminal_thread(&self) -> bool;
    /// The visible rows of the Files panel.
    fn files(&self) -> Vec<FileRow>;
    /// What the Changes panel draws.
    fn changes(&self) -> Changes;
    /// The absolute folder the Files panel shows; without it Copy Path copies the relative path.
    fn workspace_root(&self) -> Option<String> {
        None
    }
    /// Whether the focused workspace refuses writes, which hides the entries that change files.
    fn files_locked(&self) -> bool {
        false
    }
    /// Whether the workspace is on this machine, which shows the file manager entry.
    fn files_are_local(&self) -> bool {
        true
    }
    /// Whether the shell carries out `command`; the sidebar hides the entries whose command it
    /// does not.
    fn serves(
        &self,
        _command: &Command,
    ) -> bool {
        true
    }
    /// Whether `path` is one of several folders of the workspace, which offers Remove From
    /// Workspace.
    fn can_remove_folder(
        &self,
        _path: &str,
    ) -> bool {
        false
    }
    /// The merge box of the focused workspace, or `None` without a branch workspace.
    fn merge_box(&self) -> Option<MergeBox> {
        None
    }
    /// The chats whose agent is running now, on every host.
    fn running_agents(&self) -> Vec<RunningAgent> {
        Vec::new()
    }
    /// The icon `project` shows.
    fn space_icon(
        &self,
        _project: &ProjectId,
    ) -> SpaceIcon {
        SpaceIcon::Default
    }
    /// Whether the shell can generate images, which shows that entry on directories.
    fn can_generate_image(&self) -> bool {
        false
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::demo_data::DemoData;

    #[test]
    fn the_active_project_is_one_the_data_lists() {
        let data = DemoData::rich();
        let active = data.active_project().expect("an active project");
        assert!(data.projects().iter().any(|project| project.id == active));
    }

    #[test]
    fn a_loading_source_reports_pending_and_a_ready_one_ready() {
        assert_eq!(DemoData::loading().sync(), ScopeSync::Pending);
        assert_eq!(DemoData::reference().sync(), ScopeSync::Ready);
    }
}
