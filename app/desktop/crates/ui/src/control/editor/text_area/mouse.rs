//! Pointer handling of [`TextArea`]: click to place the caret, drag to select, wheel to scroll.

use gpui::{Context, MouseDownEvent, MouseUpEvent, Pixels, Point, ScrollWheelEvent, Window, point};

use crate::control::editor::field::SelectionDrag;
use crate::control::editor::text_area::TextArea;
use crate::control::editor::text_area::wrapped_element::Viewport;
use crate::control::editor::text_area::wrapped_element::clamp;
use crate::foundation::Direction;
use crate::foundation::Pose;
use crate::foundation::tokens::timing;

impl SelectionDrag for TextArea {
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
        self.pointer = position;
        let scrolling = self.extend_to_pointer();
        if scrolling && self.autoscroll.is_none() {
            self.begin_autoscroll(cx);
        }
        self.restart_blink(cx);
    }
}

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
        self.buffer.press(
            offset,
            (event.click_count, Pose::from(event.modifiers.shift)),
        );
        self.restart_blink(cx);
    }

    fn outside(
        &self,
        position: Point<Pixels>,
    ) -> Option<Direction> {
        let bounds = self.bounds?;
        if position.y < bounds.top() {
            return Some(Direction::Backward);
        }
        (position.y > bounds.bottom()).then_some(Direction::Forward)
    }

    fn extend_to_pointer(&mut self) -> bool {
        let Some(direction) = self.outside(self.pointer) else {
            self.buffer.select_to(self.offset_at(self.pointer));
            return false;
        };
        let target = self.vertical_target(direction);
        self.buffer.select_to(target);
        true
    }

    fn keep_scrolling(
        &mut self,
        cx: &mut Context<'_, Self>,
    ) -> bool {
        let wanted = self.selecting && self.extend_to_pointer();
        if !wanted {
            self.autoscroll = None;
        }
        self.restart_blink(cx);
        wanted
    }

    fn begin_autoscroll(
        &mut self,
        cx: &mut Context<'_, Self>,
    ) {
        self.autoscroll =
            Some(cx.spawn(async move |this, cx| while Self::tick(&this, cx).await {}));
    }

    async fn tick(
        this: &gpui::WeakEntity<Self>,
        cx: &mut gpui::AsyncApp,
    ) -> bool {
        cx.background_executor().timer(timing::AUTOSCROLL).await;
        this.update(cx, Self::keep_scrolling).unwrap_or(false)
    }

    pub(crate) fn on_mouse_up(
        &mut self,
        _: &MouseUpEvent,
        _: &mut Window,
        _: &mut Context<'_, Self>,
    ) {
        self.selecting = false;
        self.autoscroll = None;
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

    use crate::control::editor::test_support::{
        area::{area, area_text},
        clicks::press,
    };
    use crate::foundation::tokens::timing;

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
        cx.simulate_keystrokes("secondary-home");
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

    #[gpui::test]
    fn double_click_selects_a_word_and_triple_click_a_line(cx: &mut TestAppContext) {
        let (area, cx) = area(cx);
        area.update(cx, |area, cx| area.set_text("hello world\nsecond line", cx));
        cx.run_until_parked();
        press(cx, point(px(16.0), px(18.0)), 2);
        cx.simulate_input("X");
        assert_eq!(area_text(&area, cx), "X world\nsecond line");
        press(cx, point(px(16.0), px(18.0)), 3);
        cx.simulate_input("Y");
        assert_eq!(area_text(&area, cx), "Y\nsecond line");
    }

    #[gpui::test]
    fn a_drag_past_the_bottom_edge_keeps_selecting_row_by_row_until_the_end(
        cx: &mut TestAppContext
    ) {
        let (area, cx) = area(cx);
        area.update(cx, |area, cx| area.set_text("aa\nbb\ncc\ndd", cx));
        cx.run_until_parked();
        cx.simulate_mouse_down(
            point(px(13.0), px(18.0)),
            gpui::MouseButton::Left,
            Modifiers::none(),
        );
        cx.simulate_mouse_move(
            point(px(150.0), px(400.0)),
            gpui::MouseButton::Left,
            Modifiers::none(),
        );
        let partial = area.read_with(cx, |area, _| area.buffer.selection());
        assert!(!partial.is_empty() && partial.end < 11, "{partial:?}");
        cx.executor().advance_clock(timing::AUTOSCROLL * 8);
        cx.run_until_parked();
        let whole = area.read_with(cx, |area, _| area.buffer.selection());
        assert_eq!(whole, 0..11);
        cx.simulate_mouse_up(
            point(px(150.0), px(400.0)),
            gpui::MouseButton::Left,
            Modifiers::none(),
        );
        cx.simulate_input("Z");
        assert_eq!(area_text(&area, cx), "Z");
    }
}
