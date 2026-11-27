use gpui::{KeyBinding, actions};

use crate::control::editor::secondary::secondary;

actions!(
    ui_text_input,
    [
        Backspace,
        Delete,
        Left,
        Right,
        SelectLeft,
        SelectRight,
        SelectAll,
        Home,
        End,
        Paste,
        Cut,
        Copy,
        Submit,
        Cancel,
        WordLeft,
        WordRight,
        SelectWordLeft,
        SelectWordRight,
        DeleteWordLeft,
        DeleteWordRight,
    ]
);

pub(crate) const CONTEXT: &str = "UiTextInput";

const WORD_KEYS: [[&str; 6]; 2] = [
    [
        "ctrl-left",
        "ctrl-right",
        "ctrl-shift-left",
        "ctrl-shift-right",
        "ctrl-backspace",
        "ctrl-delete",
    ],
    [
        "alt-left",
        "alt-right",
        "alt-shift-left",
        "alt-shift-right",
        "alt-backspace",
        "alt-delete",
    ],
];

pub(crate) fn word_bindings(context: Option<&'static str>) -> Vec<KeyBinding> {
    WORD_KEYS
        .into_iter()
        .flat_map(
            |[
                left,
                right,
                select_left,
                select_right,
                delete_left,
                delete_right,
            ]| {
                [
                    KeyBinding::new(left, WordLeft, context),
                    KeyBinding::new(right, WordRight, context),
                    KeyBinding::new(select_left, SelectWordLeft, context),
                    KeyBinding::new(select_right, SelectWordRight, context),
                    KeyBinding::new(delete_left, DeleteWordLeft, context),
                    KeyBinding::new(delete_right, DeleteWordRight, context),
                ]
            },
        )
        .collect()
}

pub(crate) fn bindings() -> Vec<KeyBinding> {
    let context = Some(CONTEXT);
    let mut all = word_bindings(context);
    all.extend([
        KeyBinding::new("backspace", Backspace, context),
        KeyBinding::new("delete", Delete, context),
        KeyBinding::new("left", Left, context),
        KeyBinding::new("right", Right, context),
        KeyBinding::new("shift-left", SelectLeft, context),
        KeyBinding::new("shift-right", SelectRight, context),
        KeyBinding::new(secondary!("a"), SelectAll, context),
        KeyBinding::new(secondary!("v"), Paste, context),
        KeyBinding::new(secondary!("c"), Copy, context),
        KeyBinding::new(secondary!("x"), Cut, context),
        KeyBinding::new("home", Home, context),
        KeyBinding::new("end", End, context),
        KeyBinding::new("enter", Submit, context),
        KeyBinding::new("escape", Cancel, context),
    ]);
    all
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn every_binding_is_scoped_to_the_input() {
        let bindings = bindings();
        assert_eq!(bindings.len(), 26);
        assert!(bindings.iter().all(|binding| binding.predicate().is_some()));
    }
}
