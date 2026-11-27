use gpui::Context;

use crate::control::editor::text_input::TextInput;

impl TextInput {
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
    use gpui::{
        Entity, EntityInputHandler as _, IntoElement, ParentElement as _, Render, Styled as _,
        TestAppContext, div, point, px,
    };

    use crate::control::editor::test_support::field::{field, text_of};
    use crate::control::editor::text_input::TextInput;

    const LONG: &str = "Perf fixture: 30 turns / 4 tool calls";

    struct Narrow(Entity<TextInput>);

    impl Render for Narrow {
        fn render(
            &mut self,
            _: &mut gpui::Window,
            _: &mut gpui::Context<'_, Self>,
        ) -> impl IntoElement {
            div().w(px(80.0)).child(self.0.clone())
        }
    }

    #[gpui::test]
    fn a_select_all_scrolls_an_overflowing_field_to_the_end_as_a_browser_input(
        cx: &mut TestAppContext
    ) {
        crate::control::editor::test_support::field::install(cx);
        let (host, cx) = cx.add_window_view(|_, cx| Narrow(TextInput::build("", cx)));
        let input = host.read_with(cx, |host, _| host.0.clone());
        input.update_in(cx, |input, window, cx| {
            input.set_text(LONG, cx);
            input.focus(window, cx);
            input.select_all(cx);
        });
        cx.run_until_parked();
        let bounds = input.read_with(cx, |input, _| input.bounds.unwrap_or_default());
        let edge = point(bounds.right() - px(2.0), bounds.center().y);
        let shown_last = input.update_in(cx, |input, window, cx| {
            input.character_index_for_point(edge, window, cx)
        });
        assert!(
            shown_last.is_some_and(|index| index + 3 >= LONG.len()),
            "the field's right edge shows the end of the name: {shown_last:?}"
        );
    }

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
