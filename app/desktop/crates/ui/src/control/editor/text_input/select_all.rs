//! Selecting the whole text of a [`TextInput`] from code, as an editor does when it opens on a name.

use gpui::Context;

use crate::control::editor::text_input::TextInput;

impl TextInput {
    /// Selects the whole text, so typing replaces it.
    pub fn select_all(
        &mut self,
        cx: &mut Context<'_, Self>,
    ) {
        self.buffer.select_all();
        self.restart_blink(cx);
    }
}

#[cfg(test)]
mod tests {
    use gpui::TestAppContext;

    use crate::control::editor::test_support::field::{field, text_of};

    #[gpui::test]
    fn typing_after_a_select_all_replaces_the_text(cx: &mut TestAppContext) {
        let (input, cx) = field(cx);
        input.update(cx, |input, cx| {
            input.set_text("old name", cx);
            input.select_all(cx);
        });
        cx.simulate_input("new");
        assert_eq!(text_of(&input, cx), "new");
    }
}
