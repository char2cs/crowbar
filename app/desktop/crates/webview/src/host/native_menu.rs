use std::rc::Rc;

use crate::backend::{ChosenEntry, Platform};
use crate::error::Result;
use crate::menu::MenuRequest;
use crate::view::ParentWindow;

/// Pops the desktop's context menu up over a shell window, on the platform thread of a
/// [`Host`](crate::Host).
#[derive(Clone)]
pub struct NativeMenu {
    platform: Rc<dyn Platform>,
}

impl std::fmt::Debug for NativeMenu {
    fn fmt(
        &self,
        f: &mut std::fmt::Formatter<'_>,
    ) -> std::fmt::Result {
        f.debug_struct("NativeMenu").finish_non_exhaustive()
    }
}

impl NativeMenu {
    pub(crate) fn new(platform: Rc<dyn Platform>) -> Self {
        Self { platform }
    }

    /// Pops `request` up over `parent`; `answer` hears the chosen entry.
    ///
    /// # Errors
    /// An [`Error`](crate::Error) when the backend has stopped or cannot hold a menu over
    /// `parent`.
    pub fn popup(
        &self,
        parent: ParentWindow,
        request: MenuRequest,
        answer: ChosenEntry,
    ) -> Result<()> {
        self.platform.popup_menu(parent, request, answer)
    }
}

#[cfg(test)]
mod tests {
    use crate::backend::Call;
    use crate::backend::MockPlatform;
    use crate::host::Host;
    use crate::menu::{MenuEdge, MenuEntry};

    use super::*;

    #[test]
    fn a_host_s_menu_pops_up_on_its_platform() {
        let mock = Rc::new(MockPlatform::default());
        let host = Host::with_platform(Rc::<MockPlatform>::clone(&mock));
        let request = MenuRequest {
            entries: vec![MenuEntry::Separator],
            x: 4.0,
            y: 8.0,
            edge: MenuEdge::Start,
        };
        let parent = ParentWindow::X11(4);
        host.native_menu()
            .popup(parent, request.clone(), Box::new(|_| ()))
            .unwrap();
        assert_eq!(mock.calls(), [Call::PopupMenu { parent, request }]);
    }
}
