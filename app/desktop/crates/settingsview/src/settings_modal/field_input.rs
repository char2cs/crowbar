use gpui::{AppContext as _, Context, Entity, EventEmitter, Focusable, Subscription, Window};
use settings::{Control, Row};
use ui::{TextArea, TextAreaSize, TextInput, TextInputEvent, TextInputStyle};

use crate::frame::{Field, Fields};
use crate::intent::Intent;
use crate::intent::Runner;

fn make<Owner>(
    control: &Control,
    cx: &mut Context<'_, Owner>,
) -> Option<Field> {
    match control {
        Control::Text { placeholder, .. } => Some(line(placeholder, TextInputStyle::Compact, cx)),
        Control::Number { .. } => Some(line("", TextInputStyle::Stepper, cx)),
        Control::TextList { placeholder, .. } => Some(area(placeholder, cx)),
        _ => None,
    }
}

fn line<Owner>(
    placeholder: &str,
    style: TextInputStyle,
    cx: &mut Context<'_, Owner>,
) -> Field {
    let placeholder = placeholder.to_owned();
    Field::Line(cx.new(|cx| TextInput::new(placeholder, cx).style(style)))
}

fn area<Owner>(
    placeholder: &str,
    cx: &mut Context<'_, Owner>,
) -> Field {
    let placeholder = placeholder.to_owned();
    let build = |cx: &mut Context<'_, TextArea>| {
        TextArea::new(placeholder, cx)
            .size(TextAreaSize::Field)
            .submit_on_enter()
    };
    Field::Area(cx.new(build))
}

pub(crate) fn build<Owner: Runner>(
    fields: &mut Fields,
    content: (&Row, &str),
    (window, cx): (&mut Window, &mut Context<'_, Owner>),
) {
    let (row, text) = content;
    let Some(field) = make(row.control(), cx) else {
        return;
    };
    field.set_text(text, cx);
    let subscriptions = match &field {
        Field::Line(input) => wire(input, row, (window, cx)),
        Field::Area(area) => wire(area, row, (window, cx)),
    };
    fields.add(row.key().clone(), field, subscriptions);
}

fn wire<Owner: Runner, Editor: EventEmitter<TextInputEvent> + Focusable>(
    editor: &Entity<Editor>,
    row: &Row,
    (window, cx): (&mut Window, &mut Context<'_, Owner>),
) -> Vec<Subscription> {
    let edited = row.clone();
    let blurred = row.clone();
    let typing = cx.subscribe_in(
        editor,
        window,
        move |modal, _, event: &TextInputEvent, window, cx| {
            modal.run(Intent::Edit(edited.clone(), event.clone()), window, cx);
        },
    );
    let focus = editor.focus_handle(cx);
    let leaving = cx.on_blur(&focus, window, move |modal, window, cx| {
        modal.run(Intent::Commit(blurred.clone()), window, cx);
    });
    vec![typing, leaving]
}

#[cfg(test)]
mod tests {
    use gpui::TestAppContext;
    use settings::{Key, Read as _, Value};

    use super::Field;
    use crate::test_support::{inspect, open};

    fn focused<'a>(
        cx: &'a mut TestAppContext,
        (page, key): (&str, &str),
    ) -> (
        crate::test_support::Harness,
        &'a mut gpui::VisualTestContext,
        Field,
    ) {
        let (harness, cx) = open(cx, page);
        cx.update(|window, _| window.activate_window());
        let input = inspect(&harness, cx, |modal| {
            modal.fields.get(&Key::new(key)).cloned()
        });
        let input = input.unwrap_or_else(|| unreachable!("{key} has a field"));
        cx.update(|window, cx| input.select(window, cx));
        (harness, cx, input)
    }

    fn homepage(
        cx: &mut TestAppContext
    ) -> (
        crate::test_support::Harness,
        &mut gpui::VisualTestContext,
        Field,
    ) {
        focused(cx, ("browser", "browserHomepage"))
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
        assert_eq!(cx.update(|_, cx| input.text(cx)), "");
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
            .update_in(cx, |modal, window, cx| modal.focus.focus(window, cx));
        cx.run_until_parked();
        cx.run_until_parked();
        assert_eq!(
            harness.backend.get(&Key::new("browserHomepage")),
            Some(Value::Text("https://y.test".to_owned()))
        );
    }

    #[gpui::test]
    fn tab_leaves_a_number_field_and_clamps_what_was_typed(cx: &mut TestAppContext) {
        let (harness, cx) = open(cx, "appearance");
        cx.update(|window, _| window.activate_window());
        let input = inspect(&harness, cx, |modal| {
            modal.fields.get(&Key::new("uiFontSize")).cloned()
        })
        .unwrap();
        cx.update(|window, cx| input.select(window, cx));
        cx.simulate_keystrokes("secondary-a");
        cx.simulate_input("40");
        cx.simulate_keystrokes("tab");
        cx.run_until_parked();
        assert_eq!(
            harness.backend.get(&Key::new("uiFontSize")),
            Some(Value::Number(24.0))
        );
    }

    #[gpui::test]
    fn a_pattern_list_is_a_two_row_area_where_enter_commits_and_shift_enter_breaks(
        cx: &mut TestAppContext
    ) {
        let (harness, cx, input) = focused(cx, ("file-explorer", "hiddenFilePatterns"));
        assert!(matches!(input, Field::Area(_)), "a text area");
        cx.simulate_keystrokes("secondary-a");
        cx.simulate_input("*.a,");
        cx.simulate_keystrokes("shift-enter");
        cx.simulate_input("*.b");
        cx.simulate_keystrokes("enter");
        let committed = harness.backend.get(&Key::new("hiddenFilePatterns"));
        let list = vec!["*.a".to_owned(), "*.b".to_owned()];
        assert_eq!(committed, Some(Value::List(list)));
    }
}
