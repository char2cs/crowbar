//! Pointer handling of [`TextArea`]: click to place the caret, drag to select, wheel to scroll.

use gpui::{
    Context, MouseDownEvent, MouseMoveEvent, MouseUpEvent, Pixels, Point, ScrollWheelEvent, Window,
    point,
};

use crate::text_area::TextArea;
use crate::text_area_scroll::clamp;
use crate::viewport::Viewport;

impl TextArea {
    fn offset_at(
        &self,
        position: Point<Pixels>,
    ) -> usize {
        let Some(bounds) = self.bounds else {
            return 0;
        };
        let local = point(
            position.x - bounds.left(),
            position.y - bounds.top() + self.scroll,
        );
        self.layout.index_for_point(local)
    }

    pub(crate) fn on_mouse_down(
        &mut self,
        event: &MouseDownEvent,
        _: &mut Window,
        cx: &mut Context<'_, Self>,
    ) {
        self.selecting = true;
        let offset = self.offset_at(event.position);
        if event.modifiers.shift {
            self.buffer.select_to(offset);
            cx.notify();
            return;
        }
        self.buffer.move_to(offset);
        cx.notify();
    }

    pub(crate) fn on_mouse_move(
        &mut self,
        event: &MouseMoveEvent,
        _: &mut Window,
        cx: &mut Context<'_, Self>,
    ) {
        if !self.selecting {
            return;
        }
        self.buffer.select_to(self.offset_at(event.position));
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

    pub(crate) fn on_scroll(
        &mut self,
        event: &ScrollWheelEvent,
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) {
        let Some(bounds) = self.bounds else {
            return;
        };
        let view = Viewport {
            height: bounds.size.height,
            content: self.layout.height(),
        };
        let moved = self.scroll - event.delta.pixel_delta(window.line_height()).y;
        let target = clamp(moved, &view);
        if target == self.scroll {
            return;
        }
        self.scroll = target;
        cx.stop_propagation();
        cx.notify();
    }
}

#[cfg(test)]
mod tests {
    use gpui::{Modifiers, TestAppContext, point, px};

    use crate::test_support::{area, area_text};

    #[gpui::test]
    fn clicking_a_row_places_the_caret_on_it(cx: &mut TestAppContext) {
        let (area, cx) = area(cx);
        area.update(cx, |area, cx| area.set_text("abc\ndef", cx));
        cx.run_until_parked();
        cx.simulate_click(point(px(290.0), px(18.0)), Modifiers::none());
        cx.simulate_input("!");
        assert_eq!(area_text(&area, cx), "abc!\ndef");
        cx.simulate_click(point(px(13.0), px(38.0)), Modifiers::none());
        cx.simulate_input("?");
        assert_eq!(area_text(&area, cx), "abc!\n?def");
    }

    #[gpui::test]
    fn clicking_below_the_text_goes_to_the_last_row(cx: &mut TestAppContext) {
        let (area, cx) = area(cx);
        area.update(cx, |area, cx| area.set_text("abc\ndef", cx));
        cx.run_until_parked();
        cx.simulate_keystrokes("ctrl-home");
        cx.simulate_click(point(px(290.0), px(70.0)), Modifiers::none());
        cx.simulate_input("!");
        assert_eq!(area_text(&area, cx), "abc\ndef!");
    }

    #[gpui::test]
    fn dragging_selects_across_rows(cx: &mut TestAppContext) {
        let (area, cx) = area(cx);
        area.update(cx, |area, cx| area.set_text("abc\ndef", cx));
        cx.run_until_parked();
        cx.simulate_mouse_down(
            point(px(13.0), px(18.0)),
            gpui::MouseButton::Left,
            Modifiers::none(),
        );
        cx.simulate_mouse_move(
            point(px(290.0), px(38.0)),
            gpui::MouseButton::Left,
            Modifiers::none(),
        );
        cx.simulate_mouse_up(
            point(px(290.0), px(38.0)),
            gpui::MouseButton::Left,
            Modifiers::none(),
        );
        cx.simulate_input("X");
        assert_eq!(area_text(&area, cx), "X");
    }
}
