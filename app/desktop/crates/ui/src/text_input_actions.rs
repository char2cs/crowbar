//! Keyboard actions of a text input.

use gpui::{KeyBinding, actions};

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
    ]
);

pub(crate) const CONTEXT: &str = "UiTextInput";

pub(crate) fn bindings() -> Vec<KeyBinding> {
    let context = Some(CONTEXT);
    vec![
        KeyBinding::new("backspace", Backspace, context),
        KeyBinding::new("delete", Delete, context),
        KeyBinding::new("left", Left, context),
        KeyBinding::new("right", Right, context),
        KeyBinding::new("shift-left", SelectLeft, context),
        KeyBinding::new("shift-right", SelectRight, context),
        KeyBinding::new("secondary-a", SelectAll, context),
        KeyBinding::new("secondary-v", Paste, context),
        KeyBinding::new("secondary-c", Copy, context),
        KeyBinding::new("secondary-x", Cut, context),
        KeyBinding::new("home", Home, context),
        KeyBinding::new("end", End, context),
        KeyBinding::new("enter", Submit, context),
        KeyBinding::new("escape", Cancel, context),
    ]
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn every_binding_is_scoped_to_the_input() {
        let bindings = bindings();
        assert_eq!(bindings.len(), 14);
        assert!(bindings.iter().all(|binding| binding.predicate().is_some()));
    }
}
