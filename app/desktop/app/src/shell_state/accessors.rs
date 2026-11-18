use super::ShellState;
use crate::browser_ids::BrowserIds;
use crate::dirty_panes::DirtyPanes;
use crate::save_report::SaveReport;
use crate::serials::Serials;
use crate::terminal_ids::TerminalIds;
use crate::titles::Titles;
use layout::Layout;
use pane_host::PaneHost;

impl ShellState {
    pub(crate) fn take_notice(&mut self) -> Option<String> {
        self.notice.take()
    }

    pub(crate) fn save_reports(&self) -> async_channel::Receiver<SaveReport> {
        self.writer.reports()
    }

    pub(crate) fn layout(&self) -> &Layout {
        self.history.layout()
    }

    pub(crate) fn titles(&self) -> Titles {
        self.titles.clone()
    }

    pub(crate) fn dirty(&self) -> DirtyPanes {
        self.dirty.clone()
    }

    pub(crate) fn browsers(&self) -> BrowserIds {
        self.browsers.clone()
    }

    pub(crate) fn terminals(&self) -> TerminalIds {
        self.terminals.clone()
    }

    pub(crate) fn serials(&self) -> Serials {
        self.serials.clone()
    }

    pub(crate) fn host(&self) -> &PaneHost {
        &self.host
    }

    pub(crate) fn frozen_elements(&self) -> Vec<gpui::AnyElement> {
        self.frozen.elements()
    }
}

#[cfg(test)]
mod tests {
    use std::path::Path;
    use std::rc::Rc;
    use std::sync::Arc;

    use protocol::{PaneId, PaneTitle};
    use webview::MockPlatform;

    use super::*;
    use crate::test_support::{LAYOUT_PATH, MemoryFiles, mock_host, shell_rig};

    #[test]
    fn a_corrupt_layout_file_starts_fresh_and_leaves_a_notice() {
        let mock = Rc::new(MockPlatform::default());
        let files = Arc::new(MemoryFiles::default());
        files.seed(Path::new(LAYOUT_PATH), "garbage");
        let mut shell = ShellState::new(mock_host(&mock), files.store()).unwrap();
        assert_eq!(shell.layout().pane_count(), 0, "fresh layout");
        let notice = shell.take_notice().unwrap();
        assert!(notice.contains("layout.json.bad-"), "{notice}");
        assert!(shell.take_notice().is_none(), "shown once");
    }

    #[test]
    fn every_handle_to_the_titles_sees_the_same_titles() {
        let rig = shell_rig();
        let pane = PaneId::new("p1").unwrap();
        rig.shell
            .titles()
            .set(pane.clone(), PaneTitle::new("Fix").unwrap());
        let seen = rig.shell.titles().get(&pane).map(|title| title.to_string());
        assert_eq!(seen.as_deref(), Some("Fix"));
    }
}
