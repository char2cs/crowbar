//! Keyboard actions of a text area; editing and caret actions are shared with the text input.

use gpui::{KeyBinding, actions};

use crate::text_input_actions::{
    Backspace, Cancel, Copy, Cut, Delete, End, Home, Left, Paste, Right, SelectAll, SelectLeft,
    SelectRight, Submit,
};

actions!(
    ui_text_area,
    [
        Newline,
        Up,
        Down,
        SelectUp,
        SelectDown,
        SelectHome,
        SelectEnd,
        DocumentStart,
        DocumentEnd,
        Undo,
        Redo,
    ]
);

pub(crate) const CONTEXT: &str = "UiTextArea";

pub(crate) fn bindings() -> Vec<KeyBinding> {
    let context = Some(CONTEXT);
    vec![
        KeyBinding::new("backspace", Backspace, context),
        KeyBinding::new("delete", Delete, context),
        KeyBinding::new("left", Left, context),
        KeyBinding::new("right", Right, context),
        KeyBinding::new("up", Up, context),
        KeyBinding::new("down", Down, context),
        KeyBinding::new("shift-left", SelectLeft, context),
        KeyBinding::new("shift-right", SelectRight, context),
        KeyBinding::new("shift-up", SelectUp, context),
        KeyBinding::new("shift-down", SelectDown, context),
        KeyBinding::new("home", Home, context),
        KeyBinding::new("end", End, context),
        KeyBinding::new("shift-home", SelectHome, context),
        KeyBinding::new("shift-end", SelectEnd, context),
        KeyBinding::new("secondary-home", DocumentStart, context),
        KeyBinding::new("secondary-end", DocumentEnd, context),
        KeyBinding::new("secondary-a", SelectAll, context),
        KeyBinding::new("secondary-v", Paste, context),
        KeyBinding::new("secondary-c", Copy, context),
        KeyBinding::new("secondary-x", Cut, context),
        KeyBinding::new("secondary-z", Undo, context),
        KeyBinding::new("secondary-shift-z", Redo, context),
        KeyBinding::new("secondary-y", Redo, context),
        KeyBinding::new("enter", Newline, context),
        KeyBinding::new("secondary-enter", Submit, context),
        KeyBinding::new("escape", Cancel, context),
    ]
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn every_binding_is_scoped_to_the_text_area() {
        let bindings = bindings();
        assert!(bindings.iter().all(|binding| binding.predicate().is_some()));
        assert!(bindings.len() > 20);
    }
}
