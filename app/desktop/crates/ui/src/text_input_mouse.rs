//! Pointer handling of [`TextInput`]: click to place the caret, drag to select.

use gpui::{Context, MouseDownEvent, MouseUpEvent, Pixels, Point, Window};

use crate::pose::Pose;
use crate::selection_drag::SelectionDrag;

use crate::text_input::TextInput;

impl SelectionDrag for TextInput {
    fn selecting(&self) -> bool {
        self.selecting
    }

    fn drag_selection(
        &mut self,
        position: Point<Pixels>,
        cx: &mut Context<'_, Self>,
    ) {
        if !self.selecting {
            return;
        }
        self.buffer.select_to(self.offset_at(position));
        cx.notify();
    }
}

impl TextInput {
    fn offset_at(
        &self,
        position: Point<Pixels>,
    ) -> usize {
        let (Some(bounds), Some(line)) = (self.bounds.as_ref(), self.layout.as_ref()) else {
            return 0;
        };
        if position.y < bounds.top() {
            return 0;
        }
        if position.y > bounds.bottom() {
            return self.buffer.text().len();
        }
        line.closest_index_for_x(position.x - bounds.left())
    }

    pub(crate) fn on_mouse_down(
        &mut self,
        event: &MouseDownEvent,
        _: &mut Window,
        cx: &mut Context<'_, Self>,
    ) {
        self.selecting = true;
        let offset = self.offset_at(event.position);
        self.buffer.press(
            offset,
            (event.click_count, Pose::from(event.modifiers.shift)),
        );
        cx.notify();
    }

    pub(crate) fn on_mouse_up(
        &mut self,
        _: &MouseUpEvent,
        _: &mut Window,
        _: &mut Context<'_, Self>,
    ) {
        self.selecting = false;
    }
}

#[cfg(test)]
mod tests {
    use gpui::{Modifiers, TestAppContext, point, px};

    use crate::test_support::{field, press, text_of};

    #[gpui::test]
    fn clicking_past_the_text_puts_the_caret_at_its_end(cx: &mut TestAppContext) {
        let (input, cx) = field(cx);
        cx.simulate_input("abc");
        cx.simulate_keystrokes("home");
        cx.simulate_click(point(px(400.0), px(16.0)), Modifiers::none());
        cx.simulate_input("!");
        assert_eq!(text_of(&input, cx), "abc!");
    }

    #[gpui::test]
    fn clicking_at_the_left_edge_puts_the_caret_at_the_start(cx: &mut TestAppContext) {
        let (input, cx) = field(cx);
        cx.simulate_input("abc");
        cx.simulate_click(point(px(11.0), px(16.0)), Modifiers::none());
        cx.simulate_input("!");
        assert_eq!(text_of(&input, cx), "!abc");
    }

    #[gpui::test]
    fn double_click_selects_a_word_and_triple_click_everything(cx: &mut TestAppContext) {
        let (input, cx) = field(cx);
        cx.simulate_input("hello big world");
        press(cx, point(px(14.0), px(16.0)), 2);
        cx.simulate_input("X");
        assert_eq!(text_of(&input, cx), "X big world");
        press(cx, point(px(14.0), px(16.0)), 3);
        cx.simulate_input("Y");
        assert_eq!(text_of(&input, cx), "Y");
    }
}
