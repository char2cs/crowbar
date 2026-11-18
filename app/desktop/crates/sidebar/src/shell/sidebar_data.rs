//! What the sidebar reads from the shell.

use crate::shell::command::RowActions;
use crate::shell::git::ChangesSource;
use crate::shell::listing::FileAccess;
use crate::shell::listing::FilesSource;
use crate::shell::switcher::ActiveSpace;
use crate::shell::switcher::PaneFocus;
use crate::shell::switcher::SpaceList;

/// The read side of the sidebar, implemented by the shell over the daemon's entity streams: every
/// small source below. The view calls it on every render and never keeps a copy of what it returns.
pub trait SidebarData:
    SpaceList + ActiveSpace + PaneFocus + RowActions + FilesSource + FileAccess + ChangesSource
{
}

impl<T> SidebarData for T where
    T: SpaceList + ActiveSpace + PaneFocus + RowActions + FilesSource + FileAccess + ChangesSource
{
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::fixtures::DemoData;
    use crate::shell::switcher::ScopeSync;

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
