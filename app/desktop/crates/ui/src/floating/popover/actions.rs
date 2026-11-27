use gpui::{KeyBinding, actions};

actions!(ui_popover, [ClosePopover]);

pub(crate) const CONTEXT: &str = "UiPopover";

pub(crate) fn bindings() -> Vec<KeyBinding> {
    vec![KeyBinding::new("escape", ClosePopover, Some(CONTEXT))]
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn escape_is_bound_in_the_popover_context() {
        let bindings = bindings();
        assert_eq!(bindings.len(), 1);
        assert!(bindings[0].predicate().is_some());
    }
}
