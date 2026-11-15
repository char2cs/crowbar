//! Keyboard actions of an open menu.

use gpui::{KeyBinding, actions};

actions!(
    ui_menu,
    [
        SelectNext,
        SelectPrevious,
        Confirm,
        Dismiss,
        OpenSubmenu,
        CloseSubmenu
    ]
);

pub(crate) const CONTEXT: &str = "UiMenu";

pub(crate) fn bindings() -> Vec<KeyBinding> {
    vec![
        KeyBinding::new("down", SelectNext, Some(CONTEXT)),
        KeyBinding::new("up", SelectPrevious, Some(CONTEXT)),
        KeyBinding::new("enter", Confirm, Some(CONTEXT)),
        KeyBinding::new("right", OpenSubmenu, Some(CONTEXT)),
        KeyBinding::new("left", CloseSubmenu, Some(CONTEXT)),
        KeyBinding::new("escape", Dismiss, Some(CONTEXT)),
    ]
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn bindings_cover_the_four_actions_in_the_menu_context() {
        let bindings = bindings();
        assert_eq!(bindings.len(), 6);
        assert!(bindings.iter().all(|binding| binding.predicate().is_some()));
    }
}
