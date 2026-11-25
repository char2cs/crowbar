use crate::alert::AlertRequest;
use crate::backend::desktop::alert_asking::{AlertAsking, ChosenButton};
use crate::backend::mock::call::Call;
use crate::backend::mock::mock_platform::MockPlatform;
use crate::error::Result;
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

#[cfg(test)]
mod tests {
    use std::sync::{Arc, Mutex};

    use super::*;
    use crate::alert::{AlertButton, AlertRole};

    fn ok() -> AlertButton {
        AlertButton {
            label: "OK".into(),
            role: AlertRole::Accept,
        }
    }

    #[test]
    fn a_dialog_is_recorded_and_answers_the_button_it_was_given() {
        let mock = MockPlatform::default();
        mock.press_button(1);
        let heard = Arc::new(Mutex::new(None));
        let sink = Arc::clone(&heard);
        let answer: ChosenButton = Box::new(move |button| *sink.lock().unwrap() = button);
        let request = AlertRequest {
            title: None,
            message: "Reload?".into(),
            buttons: vec![ok()],
        };
        mock.ask_alert(ParentWindow::X11(9), request.clone(), answer)
            .unwrap();
        let recorded = Call::AskAlert {
            parent: ParentWindow::X11(9),
            request,
        };
        assert_eq!(mock.calls(), [recorded]);
        assert_eq!(*heard.lock().unwrap(), Some(1));
    }
}
