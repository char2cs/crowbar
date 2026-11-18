use std::rc::Rc;
use std::sync::Arc;

use panehost::VisiblePane;
use protocol::PaneId;
use webview::{Bounds, MockPlatform, ParentWindow};

use super::{MemoryFiles, mock_host};
use crate::core_panes::{chat_params, chat_type};
use crate::shell_state::ShellState;

pub(crate) struct ShellRig {
    pub(crate) shell: ShellState,
    pub(crate) mock: Rc<MockPlatform>,
    pub(crate) files: Arc<MemoryFiles>,
}

pub(crate) fn shell_rig() -> ShellRig {
    let mock = Rc::new(MockPlatform::default());
    let files = Arc::new(MemoryFiles::default());
    let shell = ShellState::new(mock_host(&mock), files.store()).unwrap();
    ShellRig { shell, mock, files }
}

pub(crate) fn open_chat(
    shell: &mut ShellState,
    thread: &str,
) {
    shell.open_pane(chat_type().unwrap(), chat_params(thread).unwrap());
}

fn window_area() -> Bounds {
    Bounds {
        x: 294.0,
        y: 42.0,
        width: 800.0,
        height: 600.0,
    }
}

fn visible(shell: &ShellState) -> Vec<VisiblePane> {
    let area = window_area();
    shell.layout().active_tab().map_or_else(Vec::new, |tab| {
        tab.pane_ids()
            .into_iter()
            .map(|pane| VisiblePane { pane, bounds: area })
            .collect()
    })
}

pub(crate) fn synced_chat(rig: &mut ShellRig) -> PaneId {
    open_chat(&mut rig.shell, "t1");
    let visible = visible(&rig.shell);
    rig.shell.sync(ParentWindow::X11(9), visible);
    rig.shell.layout().active_tab().unwrap().pane_ids()[0].clone()
}
