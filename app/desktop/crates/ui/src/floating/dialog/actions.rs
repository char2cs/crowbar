//! The keys of a dialog: `Escape` closes it, `Tab` and `Shift-Tab` move focus inside it.

use gpui::{KeyBinding, actions};

actions!(ui_dialog, [CloseDialog, FocusNext, FocusPrevious]);

pub(crate) const CONTEXT: &str = "UiDialog";

pub(crate) fn bindings() -> Vec<KeyBinding> {
    vec![
        KeyBinding::new("escape", CloseDialog, Some(CONTEXT)),
        KeyBinding::new("tab", FocusNext, Some(CONTEXT)),
        KeyBinding::new("shift-tab", FocusPrevious, Some(CONTEXT)),
    ]
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn every_key_is_bound_in_the_dialog_context() {
        assert!(
            bindings()
                .iter()
                .all(|binding| binding.predicate().is_some())
        );
    }
}
