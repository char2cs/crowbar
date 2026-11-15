//! Dragging the scrollbar thumb with the pointer.

use std::cell::Cell;
use std::rc::Rc;

use gpui::{
    Bounds, MouseButton, MouseDownEvent, MouseMoveEvent, MouseUpEvent, Pixels, ScrollHandle,
    Window, point, px,
};

use crate::thumb::Thumb;

type Grab = (f32, f32);

#[derive(Debug)]
pub(crate) struct ThumbDrag {
    handle: ScrollHandle,
    view: f32,
    thumb: Bounds<Pixels>,
    grab: Cell<Option<Grab>>,
}

impl ThumbDrag {
    pub(crate) fn new(
        handle: ScrollHandle,
        view: f32,
        thumb: Bounds<Pixels>,
    ) -> Self {
        Self {
            handle,
            view,
            thumb,
            grab: Cell::new(None),
        }
    }

    pub(crate) fn target(
        &self,
        grab: Grab,
        pointer_y: f32,
    ) -> f32 {
        let max = f32::from(self.handle.max_offset().height);
        Thumb::scrolled_after(self.view, max, grab.1, pointer_y - grab.0)
    }

    fn press(
        &self,
        event: &MouseDownEvent,
        window: &mut Window,
    ) {
        if event.button != MouseButton::Left || !self.thumb.contains(&event.position) {
            return;
        }
        let scrolled = -f32::from(self.handle.offset().y);
        self.grab.set(Some((f32::from(event.position.y), scrolled)));
        window.prevent_default();
    }

    fn pull(
        &self,
        event: &MouseMoveEvent,
        window: &mut Window,
    ) {
        let Some(grab) = self.grab.get() else {
            return;
        };
        let target = self.target(grab, f32::from(event.position.y));
        self.handle
            .set_offset(point(self.handle.offset().x, px(-target)));
        window.refresh();
    }

    pub(crate) fn install(
        self,
        window: &mut Window,
    ) {
        let drag = Rc::new(self);
        let (pressed, moved, released) = (Rc::clone(&drag), Rc::clone(&drag), drag);
        window.on_mouse_event(move |event: &MouseDownEvent, _, window, _| {
            pressed.press(event, window);
        });
        window.on_mouse_event(move |event: &MouseMoveEvent, _, window, _| {
            moved.pull(event, window);
        });
        window.on_mouse_event(move |_: &MouseUpEvent, _, _, _| released.grab.set(None));
    }
}

#[cfg(test)]
mod tests {
    use gpui::{ScrollHandle, point, px, size};

    use super::*;

    #[test]
    fn dragging_the_thumb_scrolls_proportionally_and_clamps() {
        let handle = ScrollHandle::new();
        let drag = ThumbDrag::new(
            handle,
            100.0,
            Bounds::new(point(px(0.0), px(0.0)), size(px(6.0), px(20.0))),
        );
        assert!(
            drag.target((10.0, 0.0), 10.0).abs() < f32::EPSILON,
            "no content, nothing to scroll"
        );
    }
}
