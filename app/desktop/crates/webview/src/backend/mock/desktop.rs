use crate::alert::AlertRequest;
use crate::backend::desktop::alert_asking::{AlertAsking, ChosenButton};
use crate::backend::desktop::caret_blink::CaretReport;
use crate::backend::desktop::color_scheme::ColorScheme;
use crate::backend::desktop::desktop_settings::DesktopSettings;
use crate::backend::desktop::file_choosing::{ChosenFiles, FileChoosing};
use crate::backend::desktop::menu_popping::{ChosenEntry, MenuPopping};
use crate::backend::mock::call::Call;
use crate::backend::mock::mock_platform::MockPlatform;
use crate::backend::view::browsing_data::BrowsingData;
use crate::chooser::FileChoice;
use crate::error::Result;
use crate::menu::MenuRequest;
use crate::view::ParentWindow;

impl AlertAsking for MockPlatform {
    fn ask_alert(
        &self,
        parent: ParentWindow,
        request: AlertRequest,
        answer: ChosenButton,
    ) -> Result<()> {
        self.record(Call::AskAlert { parent, request });
        answer(self.pressed.get());
        Ok(())
    }
}

impl FileChoosing for MockPlatform {
    fn choose_files(
        &self,
        parent: ParentWindow,
        choice: FileChoice,
        answer: ChosenFiles,
    ) -> Result<()> {
        self.record(Call::ChooseFiles { parent, choice });
        answer(self.chosen.borrow().clone());
        Ok(())
    }
}

impl MenuPopping for MockPlatform {
    fn popup_menu(
        &self,
        parent: ParentWindow,
        request: MenuRequest,
        answer: ChosenEntry,
    ) -> Result<()> {
        self.record(Call::PopupMenu { parent, request });
        answer(self.picked.borrow().clone());
        Ok(())
    }
}

impl BrowsingData for MockPlatform {
    fn clear_browsing_data(&self) -> Result<()> {
        self.record(Call::ClearBrowsingData);
        Ok(())
    }
}

impl DesktopSettings for MockPlatform {
    fn follow_caret(
        &self,
        _report: CaretReport,
    ) -> Result<()> {
        self.record(Call::FollowCaret);
        Ok(())
    }

    fn draw_widgets_in(
        &self,
        scheme: ColorScheme,
    ) -> Result<()> {
        self.record(Call::DrawWidgetsIn { scheme });
        Ok(())
    }
}

#[cfg(test)]
mod tests {
    use std::path::PathBuf;
    use std::sync::{Arc, Mutex};

    use super::*;
    use crate::chooser::ChoiceKind;
    use crate::menu::MenuEdge;

    type Heard = Arc<Mutex<(Option<usize>, Vec<PathBuf>, Option<Vec<usize>>)>>;

    const PARENT: ParentWindow = ParentWindow::X11(9);
    const FOLDER: FileChoice = FileChoice {
        title: "Select Folder",
        kind: ChoiceKind::Folder,
        filter: None,
    };
    const MENU: MenuRequest = MenuRequest {
        entries: vec![],
        x: 1.0,
        y: 2.0,
        edge: MenuEdge::Start,
    };

    fn ask_all(
        mock: &MockPlatform,
        heard: &Heard,
    ) {
        let (button, paths, entry) = (Arc::clone(heard), Arc::clone(heard), Arc::clone(heard));
        let alert = AlertRequest {
            title: None,
            message: "Reload?".into(),
            buttons: vec![],
        };
        let pressed: ChosenButton = Box::new(move |b| button.lock().unwrap().0 = b);
        let chosen: ChosenFiles = Box::new(move |p| paths.lock().unwrap().1 = p);
        let picked: ChosenEntry = Box::new(move |e| entry.lock().unwrap().2 = e);
        mock.ask_alert(PARENT, alert, pressed).unwrap();
        mock.choose_files(PARENT, FOLDER, chosen).unwrap();
        mock.popup_menu(PARENT, MENU, picked).unwrap();
    }

    #[test]
    fn the_desktop_widgets_answer_what_the_test_gave_them() {
        let mock = MockPlatform::default();
        let heard: Heard = Arc::new(Mutex::new((Some(9), vec![PathBuf::new()], Some(vec![]))));
        ask_all(&mock, &heard);
        assert_eq!(*heard.lock().unwrap(), (None, vec![], None), "dismissed");
        mock.press_button(1);
        mock.choose_paths(vec![PathBuf::from("/w/app")]);
        mock.pick_entry(vec![2, 0]);
        ask_all(&mock, &heard);
        let chosen = (Some(1), vec![PathBuf::from("/w/app")], Some(vec![2, 0]));
        assert_eq!(*heard.lock().unwrap(), chosen, "answered");
        assert_eq!(mock.calls().len(), 6, "every widget recorded");
    }
}
