//! Pointer handling of [`TextInput`]: click to place the caret, drag to select.

use gpui::{Context, MouseDownEvent, MouseMoveEvent, MouseUpEvent, Pixels, Point, Window};

use crate::text_input::TextInput;

impl TextInput {
    fn offset_at(&self, position: Point<Pixels>) -> usize {
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
        if event.modifiers.shift {
            self.buffer.select_to(offset);
        } else {
            self.buffer.move_to(offset);
        }
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
}

#[cfg(test)]
#[path = "text_input_mouse_tests.rs"]
mod tests;
