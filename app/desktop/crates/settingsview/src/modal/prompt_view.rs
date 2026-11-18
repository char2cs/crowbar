//! The confirmation dialog.

use gpui::{IntoElement, ParentElement, Styled, div};
use settings::Prompt;
use ui::{Button, ButtonSize, ButtonVariant, Dialog};

use crate::frame::spacing;
use crate::intent::Intent;
use crate::intent::IntentSink;

pub(crate) fn prompt_view(
    sink: &IntentSink,
    prompt: &Prompt,
) -> impl IntoElement {
    let buttons = div()
        .flex()
        .justify_end()
        .gap(spacing::COMFORTABLE)
        .child(
            Button::new("prompt-cancel", "Cancel")
                .variant(ButtonVariant::Outline)
                .size(ButtonSize::Sm)
                .on_click(sink.send(Intent::Dismiss)),
        )
        .child(
            Button::new("prompt-accept", prompt.confirm.action.clone())
                .variant(ButtonVariant::Destructive)
                .size(ButtonSize::Sm)
                .on_click(sink.send(Intent::Accept)),
        );
    let closer = sink.clone();
    Dialog::new("settings-confirm")
        .title(prompt.title().to_owned())
        .body(prompt.message().to_owned())
        .footer(buttons)
        .on_close(move |window, cx| closer.deliver(Intent::Dismiss, (window, cx)))
}

#[cfg(test)]
mod tests {
    use gpui::TestAppContext;
    use settings::Control;

    use crate::intent::Intent;
    use crate::test_support::{inspect, open, row, run};

    fn clear_data() -> settings::ButtonSpec {
        let Control::Button(spec) = row("browserClearData").control else {
            panic!("browserClearData is a button row");
        };
        spec
    }

    #[gpui::test]
    fn a_destructive_button_asks_and_only_the_accept_runs_it(cx: &mut TestAppContext) {
        let (harness, cx) = open(cx, "browser");
        run(&harness, Intent::Button(clear_data()), cx);
        assert!(inspect(&harness, cx, |modal| modal
            .state
            .prompt()
            .is_some()));
        run(&harness, Intent::Dismiss, cx);
        assert_eq!(harness.backend.invoked().len(), 0);
        run(&harness, Intent::Button(clear_data()), cx);
        run(&harness, Intent::Accept, cx);
        assert_eq!(harness.backend.invoked().len(), 1);
        assert!(inspect(&harness, cx, |modal| modal
            .state
            .prompt()
            .is_none()));
    }
}
