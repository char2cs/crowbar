//! Asks the settings' confirmations as the desktop's message dialog, when the shell installed one.

use gpui::{Context, TaskExt as _, Window};
use ui::{AlertRequest, NativeAlert};

use crate::intent::Intent;
use crate::intent::Runner as _;
use crate::modal::prompt_request;
use crate::settings_modal::SettingsModal;

impl SettingsModal {
    pub(crate) fn ask_natively(
        &self,
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) {
        let (Some(native), Some(prompt)) = (NativeAlert::installed(cx), self.state.prompt()) else {
            return;
        };
        let answer = native.ask(prompt_request(prompt), window, cx);
        cx.spawn_in(window, async move |this, cx| {
            let intent = match answer.await {
                Some(AlertRequest::GO) => Intent::Accept,
                _ => Intent::Dismiss,
            };
            this.update_in(cx, |this, window, cx| this.run(intent, window, cx))
        })
        .detach_and_log_err(cx);
    }
}

#[cfg(test)]
mod tests {
    use std::cell::RefCell;
    use std::rc::Rc;

    use gpui::TestAppContext;
    use settings::Control;

    use crate::intent::Intent;
    use crate::test_support::{inspect, open, row, run};

    fn clear_data() -> settings::ButtonSpec {
        let clear = row("browserClearData");
        let Control::Button(spec) = clear.control() else {
            panic!("browserClearData is a button row");
        };
        spec.clone()
    }

    #[gpui::test]
    fn with_a_native_dialog_the_question_is_asked_there_and_its_accept_runs_it(
        cx: &mut TestAppContext
    ) {
        let asked: Rc<RefCell<Vec<ui::AlertRequest>>> = Rc::default();
        let heard = Rc::clone(&asked);
        let ask: ui::AskAlert = Rc::new(move |request, _, _| {
            heard.borrow_mut().push(request);
            gpui::Task::ready(Some(1))
        });
        cx.update(|cx| cx.set_global(ui::NativeAlert::new(ask)));
        let (harness, cx) = open(cx, "browser");
        run(&harness, Intent::Button(clear_data()), cx);
        cx.run_until_parked();
        let labels: Vec<Vec<String>> = asked
            .borrow()
            .iter()
            .map(|request| {
                request
                    .buttons
                    .iter()
                    .map(|b| b.label.to_string())
                    .collect()
            })
            .collect();
        assert_eq!(labels.len(), 1, "asked once");
        assert_eq!(labels[0].first().map(String::as_str), Some("Cancel"));
        assert_eq!(harness.backend.invoked().len(), 1, "the accept ran it");
        assert!(inspect(&harness, cx, |modal| modal
            .state
            .prompt()
            .is_none()));
    }
}
