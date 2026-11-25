//! The desktop's own choosers, menus and dialogs, shown on a host's platform thread.

use std::rc::Rc;

use crate::host::{FileChooser, Host, NativeAlert, NativeMenu};

impl Host {
    /// The desktop's file chooser, opened on this host's platform thread.
    #[must_use]
    pub fn file_chooser(&self) -> FileChooser {
        FileChooser::new(Rc::clone(&self.platform))
    }

    /// The desktop's context menu, popped up on this host's platform thread.
    #[must_use]
    pub fn native_menu(&self) -> NativeMenu {
        NativeMenu::new(Rc::clone(&self.platform))
    }

    /// The desktop's message dialog, asked on this host's platform thread.
    #[must_use]
    pub fn native_alert(&self) -> NativeAlert {
        NativeAlert::new(Rc::clone(&self.platform))
    }
}

#[cfg(test)]
mod tests {
    use crate::alert::AlertRequest;
    use crate::backend::{Call, MockPlatform};
    use crate::chooser::{ChoiceKind, FileChoice};
    use crate::menu::{MenuEdge, MenuRequest};
    use crate::view::ParentWindow;

    use super::*;

    const CHOICE: FileChoice = FileChoice {
        title: "Open File",
        kind: ChoiceKind::File,
        filter: None,
    };

    const MENU: MenuRequest = MenuRequest {
        entries: vec![],
        x: 0.0,
        y: 0.0,
        edge: MenuEdge::End,
    };

    fn alert() -> AlertRequest {
        AlertRequest {
            title: None,
            message: "Reload?".into(),
            buttons: vec![],
        }
    }

    #[test]
    fn a_host_s_chooser_menu_and_dialog_all_reach_its_platform() {
        let mock = Rc::new(MockPlatform::default());
        let host = Host::with_platform(Rc::<MockPlatform>::clone(&mock));
        let parent = ParentWindow::X11(4);
        let (choice, menu, alert) = (CHOICE, MENU, alert());
        host.file_chooser()
            .choose(parent, choice, Box::new(|_| ()))
            .unwrap();
        host.native_menu()
            .popup(parent, menu.clone(), Box::new(|_| ()))
            .unwrap();
        host.native_alert()
            .ask(parent, alert.clone(), Box::new(|_| ()))
            .unwrap();
        let want = [
            Call::ChooseFiles { parent, choice },
            Call::PopupMenu {
                parent,
                request: menu,
            },
            Call::AskAlert {
                parent,
                request: alert,
            },
        ];
        assert_eq!(mock.calls(), want);
    }
}
