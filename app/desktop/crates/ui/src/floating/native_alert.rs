//! The desktop's own message dialog, which the shell installs at start in place of drawn confirms.

mod alert_button;
mod alert_request;
mod alert_role;

use std::rc::Rc;

use gpui::{App, Global, Task, Window};

pub use alert_button::AlertButton;
pub use alert_request::AlertRequest;
pub use alert_role::AlertRole;

use crate::foundation::opaque_debug;

/// Asks an [`AlertRequest`] over a window; its task ends with the pressed button's index, none
/// when the dialog is dismissed.
pub type AskAlert = Rc<dyn Fn(AlertRequest, &Window, &mut App) -> Task<Option<usize>>>;

/// The installed native message dialog, a global; confirms are drawn in the window until one is
/// installed.
#[derive(Clone)]
pub struct NativeAlert(AskAlert);

opaque_debug!(NativeAlert);

impl Global for NativeAlert {}

impl NativeAlert {
    /// A native message dialog asked with `ask`.
    #[must_use]
    pub fn new(ask: AskAlert) -> Self {
        Self(ask)
    }

    /// The installed native message dialog, if the shell installed one.
    #[must_use]
    pub fn installed(cx: &App) -> Option<Self> {
        cx.try_global::<Self>().cloned()
    }

    /// Asks `request` over `window`; the task ends with the pressed button.
    pub fn ask(
        &self,
        request: AlertRequest,
        window: &Window,
        cx: &mut App,
    ) -> Task<Option<usize>> {
        (self.0)(request, window, cx)
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
        let pressed = window.update(|window, cx| {
            let native = NativeAlert::installed(cx);
            native.map(|native| native.ask(request.clone(), window, cx))
        });
        let pressed = pressed.map(|task| window.foreground_executor().block_test(task));
        assert_eq!(pressed, Some(Some(1)));
        assert_eq!(*heard.borrow(), [request]);
    }
}
