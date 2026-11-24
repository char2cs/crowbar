use std::rc::Rc;
use std::sync::Arc;

use panehost::VisiblePane;
use protocol::PaneId;
use webview::{Bounds, MockPlatform, ParentWindow};

use super::{FakeSpace, MemoryFiles, mock_host};
use crate::core_panes::{chat_params, chat_type};
use crate::shell_state::ShellState;

pub(crate) struct ShellRig {
    pub(crate) shell: ShellState,
    pub(crate) mock: Rc<MockPlatform>,
    pub(crate) files: Arc<MemoryFiles>,
    pub(crate) space: Rc<FakeSpace>,
}

pub(crate) fn shell_rig() -> ShellRig {
    let mock = Rc::new(MockPlatform::default());
    let files = Arc::new(MemoryFiles::default());
    let space = Rc::new(FakeSpace::default());
    let port = Rc::<FakeSpace>::clone(&space);
    let shell = ShellState::new(mock_host(&mock), files.store(), port).unwrap();
    ShellRig {
        shell,
        mock,
        files,
        space,
    }
}

pub(crate) fn open_chat(
    shell: &mut ShellState,
    thread: &str,
) {
    let nobody = PaneId::new("nobody").unwrap();
    let chat = protocol::PaneOpen {
        type_id: chat_type().unwrap(),
        params: chat_params(thread).unwrap(),
        file: None,
        mode: protocol::OpenMode::Kept,
    };
    let opened = shell.requested_open(&nobody, chat);
    shell.apply(opened);
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
    shell.showing().map_or_else(Vec::new, |tab| {
        tab.pane_ids()
            .into_iter()
            .map(|pane| VisiblePane { pane, bounds: area })
            .collect()
    })
}

/// Mounts every pane of the active tab, as a frame of the window would.
pub(crate) fn synced_chat(rig: &mut ShellRig) -> PaneId {
    open_chat(&mut rig.shell, "t1");
    resync(rig);
    rig.shell.showing().unwrap().pane_ids()[0].clone()
}

/// Drives every visible pane of the rig's layout, as a frame does.
pub(crate) fn resync(rig: &mut ShellRig) {
    sync_shown(&mut rig.shell);
}

pub(crate) fn sync_shown(shell: &mut ShellState) {
    let visible = visible(shell);
    shell.sync(ParentWindow::X11(9), (visible, &()));
}
