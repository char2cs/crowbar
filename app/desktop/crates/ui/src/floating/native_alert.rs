mod alert_button;
mod alert_request;
mod alert_role;

use std::rc::Rc;

use gpui::{App, Global, Task, Window};

pub use alert_button::AlertButton;
pub use alert_request::AlertRequest;
pub use alert_role::AlertRole;

use crate::foundation::opaque_debug;

pub type AskAlert = Rc<dyn Fn(AlertRequest, &Window, &mut App) -> Task<Option<usize>>>;

#[derive(Clone)]
pub struct NativeAlert(AskAlert);

opaque_debug!(NativeAlert);

impl Global for NativeAlert {}

impl NativeAlert {
    #[must_use]
    pub fn new(ask: AskAlert) -> Self {
        Self(ask)
    }

    pub fn ask(
        request: AlertRequest,
        window: &Window,
        cx: &mut App,
    ) -> Task<Option<usize>> {
        let Some(native) = cx.try_global::<Self>().cloned() else {
            tracing::error!(message = %request.message, "no message dialog to ask with; cancelled");
            return Task::ready(None);
        };
        (native.0)(request, window, cx)
    }
}

#[cfg(test)]
mod tests {
    use std::cell::RefCell;

    use gpui::TestAppContext;

    use super::*;

    #[gpui::test]
    fn an_installed_dialog_hears_what_is_asked_and_answers_the_pressed_button(
        cx: &mut TestAppContext
    ) {
        let heard: Rc<RefCell<Vec<AlertRequest>>> = Rc::default();
        let log = Rc::clone(&heard);
        let ask: AskAlert = Rc::new(move |request, _, _| {
            log.borrow_mut().push(request);
            Task::ready(Some(1))
        });
        cx.update(|cx| cx.set_global(NativeAlert::new(ask)));
        let request = AlertRequest {
            title: None,
            message: "Reload?".into(),
            buttons: vec![AlertButton::new("OK", AlertRole::Accept)],
        };
        let (_, window) = cx.add_window_view(|_, _| gpui::Empty);
        let pressed = window.update(|window, cx| NativeAlert::ask(request.clone(), window, cx));
        let pressed = window.foreground_executor().block_test(pressed);
        assert_eq!(pressed, Some(1));
        assert_eq!(*heard.borrow(), [request]);
    }

    #[gpui::test]
    fn with_no_dialog_installed_a_question_counts_as_cancelled(cx: &mut TestAppContext) {
        let request = AlertRequest {
            title: None,
            message: "Reload?".into(),
            buttons: vec![AlertButton::new("OK", AlertRole::Accept)],
        };
        let (_, window) = cx.add_window_view(|_, _| gpui::Empty);
        let pressed = window.update(|window, cx| NativeAlert::ask(request, window, cx));
        assert_eq!(window.foreground_executor().block_test(pressed), None);
    }
}
