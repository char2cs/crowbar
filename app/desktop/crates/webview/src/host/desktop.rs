use std::rc::Rc;

use crate::alert::AlertRequest;
use crate::backend::{ChosenButton, ChosenEntry, ChosenFiles, ColorScheme, Platform};
use crate::chooser::FileChoice;
use crate::error::Result;
use crate::menu::MenuRequest;
use crate::view::ParentWindow;

/// The desktop's own file chooser, context menu and message dialog, shown over a shell window on
/// the platform thread of a [`Host`](crate::Host).
#[derive(Clone)]
pub struct Desktop {
    platform: Rc<dyn Platform>,
}

impl std::fmt::Debug for Desktop {
    fn fmt(
        &self,
        f: &mut std::fmt::Formatter<'_>,
    ) -> std::fmt::Result {
        f.debug_struct("Desktop").finish_non_exhaustive()
    }
}

impl Desktop {
    pub(crate) fn new(platform: Rc<dyn Platform>) -> Self {
        Self { platform }
    }

    /// Opens a chooser for `choice` over `parent`; `answer` hears the chosen paths.
    ///
    /// # Errors
    /// An [`Error`](crate::Error) when the backend has stopped or cannot show it over `parent`.
    pub fn choose(
        &self,
        parent: ParentWindow,
        choice: FileChoice,
        answer: ChosenFiles,
    ) -> Result<()> {
        self.platform.choose_files(parent, choice, answer)
    }

    /// Pops `request` up over `parent`; `answer` hears the chosen entry.
    ///
    /// # Errors
    /// An [`Error`](crate::Error) when the backend has stopped or cannot show it over `parent`.
    pub fn popup(
        &self,
        parent: ParentWindow,
        request: MenuRequest,
        answer: ChosenEntry,
    ) -> Result<()> {
        self.platform.popup_menu(parent, request, answer)
    }

    /// Asks `request` over `parent`; `answer` hears the pressed button.
    ///
    /// # Errors
    /// An [`Error`](crate::Error) when the backend has stopped or cannot show it over `parent`.
    pub fn ask(
        &self,
        parent: ParentWindow,
        request: AlertRequest,
        answer: ChosenButton,
    ) -> Result<()> {
        self.platform.ask_alert(parent, request, answer)
    }

    /// Draws the chooser, menus and dialogs in `scheme` from now on.
    ///
    /// # Errors
    /// An [`Error`](crate::Error) when the backend has stopped.
    pub fn draw_widgets_in(
        &self,
        scheme: ColorScheme,
    ) -> Result<()> {
        self.platform.draw_widgets_in(scheme)
    }
}

#[cfg(test)]
mod tests {
    use crate::backend::{Call, MockPlatform};
    use crate::chooser::ChoiceKind;
    use crate::host::Host;
    use crate::menu::MenuEdge;

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
    fn a_host_s_chooser_menu_dialog_and_their_scheme_all_reach_its_platform() {
        let mock = Rc::new(MockPlatform::default());
        let desktop = Host::with_platform(Rc::<MockPlatform>::clone(&mock)).desktop();
        let parent = ParentWindow::X11(4);
        desktop.choose(parent, CHOICE, Box::new(|_| ())).unwrap();
        desktop.popup(parent, MENU, Box::new(|_| ())).unwrap();
        desktop.ask(parent, alert(), Box::new(|_| ())).unwrap();
        desktop.draw_widgets_in(ColorScheme::Dark).unwrap();
        let want = [
            Call::ChooseFiles {
                parent,
                choice: CHOICE,
            },
            Call::PopupMenu {
                parent,
                request: MENU,
            },
            Call::AskAlert {
                parent,
                request: alert(),
            },
            Call::DrawWidgetsIn {
                scheme: ColorScheme::Dark,
            },
        ];
        assert_eq!(mock.calls(), want);
    }
}
