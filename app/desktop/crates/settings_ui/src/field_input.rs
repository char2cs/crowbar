//! Builds the text input of a row and wires it to the modal.

use gpui::{AppContext as _, Context, Focusable as _, Window};
use ui::{TextInput, TextInputEvent, TextInputStyle};

use crate::control::Control;
use crate::fields::Fields;
use crate::intent::Intent;
use crate::row::Row;
use crate::settings_modal::SettingsModal;

fn placeholder_and_style(control: &Control) -> Option<(&str, TextInputStyle)> {
    match control {
        Control::Text { placeholder, .. } | Control::TextList { placeholder, .. } => {
            Some((placeholder, TextInputStyle::Compact))
        },
        Control::Number { .. } => Some(("", TextInputStyle::Stepper)),
        _ => None,
    }
}

pub(crate) fn build(
    fields: &mut Fields,
    (row, text): (&Row, &str),
    (window, cx): (&mut Window, &mut Context<'_, SettingsModal>),
) {
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
#[path = "field_input_tests.rs"]
mod tests;
