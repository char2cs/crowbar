use crate::backend::menu_popping::{ChosenEntry, MenuPopping};
use crate::backend::mock::call::Call;
use crate::backend::mock::mock_platform::MockPlatform;
use crate::error::Result;
use crate::menu::MenuRequest;
use crate::view::ParentWindow;

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

#[cfg(test)]
mod tests {
    use std::sync::{Arc, Mutex};

    use super::*;
    use crate::menu::{MenuEdge, MenuEntry};

    #[test]
    fn a_menu_is_recorded_and_answers_the_entry_it_was_given() {
        let mock = MockPlatform::default();
        mock.pick_entry(vec![2, 0]);
        let heard = Arc::new(Mutex::new(None));
        let sink = Arc::clone(&heard);
        let answer: ChosenEntry = Box::new(move |path| *sink.lock().unwrap() = path);
        let request = MenuRequest {
            entries: vec![MenuEntry::Separator],
            x: 1.0,
            y: 2.0,
            edge: MenuEdge::Start,
        };
        mock.popup_menu(ParentWindow::X11(9), request.clone(), answer)
            .unwrap();
        let recorded = Call::PopupMenu {
            parent: ParentWindow::X11(9),
            request,
        };
        assert_eq!(mock.calls(), [recorded]);
        assert_eq!(*heard.lock().unwrap(), Some(vec![2, 0]));
    }
}
