//! Builds the text input of a row and wires it to the modal.

use gpui::{AppContext as _, Context, Focusable as _, Window};
use settings::{Control, Row};
use ui::{TextInput, TextInputEvent, TextInputStyle};

use crate::fields::Fields;
use crate::intent::Intent;
use crate::intent_runner::IntentRunner;

fn placeholder_and_style(control: &Control) -> Option<(&str, TextInputStyle)> {
    match control {
        Control::Text { placeholder, .. } | Control::TextList { placeholder, .. } => {
            Some((placeholder, TextInputStyle::Compact))
        },
        Control::Number { .. } => Some(("", TextInputStyle::Stepper)),
        _ => None,
    }
}

pub(crate) fn build<Owner: IntentRunner>(
    fields: &mut Fields,
    content: (&Row, &str),
    (window, cx): (&mut Window, &mut Context<'_, Owner>),
) {
    let (row, text) = content;
    let Some((placeholder, style)) = placeholder_and_style(&row.control) else {
        return;
    };
    let input = cx.new(|cx| TextInput::new(placeholder.to_owned(), cx).style(style));
    input.update(cx, |input, cx| input.set_text(text, cx));
    let edited = row.clone();
    let blurred = row.clone();
    let typing = cx.subscribe_in(
        &input,
        window,
        move |modal, _, event: &TextInputEvent, window, cx| {
            modal.run(Intent::Edit(edited.clone(), event.clone()), window, cx);
        },
    );
    let focus = input.focus_handle(cx);
    let leaving = cx.on_blur(&focus, window, move |modal, window, cx| {
        modal.run(Intent::Commit(blurred.clone()), window, cx);
    });
    fields.add(row.key.clone(), input, vec![typing, leaving]);
}

#[cfg(test)]
mod tests {
    use gpui::TestAppContext;
    use settings::{Key, SettingsRead as _, Value};

    use crate::test_support::{inspect, open};

    fn homepage(
        cx: &mut TestAppContext
    ) -> (
        crate::test_support::Harness,
        &mut gpui::VisualTestContext,
        gpui::Entity<ui::TextInput>,
    ) {
        let (harness, cx) = open(cx, "browser");
        cx.update(|window, _| window.activate_window());
        let input = inspect(&harness, cx, |modal| {
            modal.fields.get(&Key::new("browserHomepage")).cloned()
        })
        .unwrap();
        cx.update(|window, cx| input.read(cx).focus(window));
        (harness, cx, input)
    }

    #[gpui::test]
    fn enter_commits_the_typed_text(cx: &mut TestAppContext) {
        let (harness, cx, _) = homepage(cx);
        cx.simulate_input("https://x.test");
        assert_eq!(harness.backend.get(&Key::new("browserHomepage")), None);
        cx.simulate_keystrokes("enter");
        assert_eq!(
            harness.backend.get(&Key::new("browserHomepage")),
            Some(Value::Text("https://x.test".to_owned()))
        );
    }

    #[gpui::test]
    fn escape_throws_the_draft_away(cx: &mut TestAppContext) {
        let (harness, cx, input) = homepage(cx);
        cx.simulate_input("https://x.test");
        cx.simulate_keystrokes("escape");
        assert_eq!(harness.backend.get(&Key::new("browserHomepage")), None);
        assert_eq!(input.read_with(cx, |input, _| input.text().to_owned()), "");
    }

    #[gpui::test]
    fn leaving_the_field_commits_it(cx: &mut TestAppContext) {
        let (harness, cx, _) = homepage(cx);
        cx.simulate_input("https://y.test");
        let before = inspect(&harness, cx, |modal| {
            modal
                .state
                .is_dirty(&crate::test_support::row("browserHomepage"))
        });
        assert!(before, "typing is a draft");
        harness
            .modal
            .update_in(cx, |modal, window, _| modal.focus.focus(window));
        cx.run_until_parked();
        cx.run_until_parked();
        assert_eq!(
            harness.backend.get(&Key::new("browserHomepage")),
            Some(Value::Text("https://y.test".to_owned()))
        );
    }
}
