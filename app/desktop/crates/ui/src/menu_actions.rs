//! Keyboard actions of an open menu.

use gpui::{KeyBinding, actions};

actions!(ui_menu, [SelectNext, SelectPrevious, Confirm, Dismiss]);

pub(crate) const CONTEXT: &str = "UiMenu";

pub(crate) fn bindings() -> Vec<KeyBinding> {
    vec![
        KeyBinding::new("down", SelectNext, Some(CONTEXT)),
        KeyBinding::new("up", SelectPrevious, Some(CONTEXT)),
        KeyBinding::new("enter", Confirm, Some(CONTEXT)),
        KeyBinding::new("escape", Dismiss, Some(CONTEXT)),
    ]
}

#[cfg(test)]
#[path = "menu_actions_tests.rs"]
mod tests;
