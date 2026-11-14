use std::time::Instant;

use gpui::{
    Context, MouseButton, MouseDownEvent, MouseMoveEvent, MouseUpEvent, Pixels, Point, Window,
};
use model::Command;
use ui::Motion;

use crate::drag_session::DragSession;
use crate::press::Press;
use crate::settle::Settle;
use crate::strip_drag::StripDrag;
use crate::strip_metrics;
use crate::tab_bar::TabBar;
use crate::tab_bar_event::TabBarEvent;

impl TabBar {
    pub(crate) fn pressed(
        &mut self,
        index: usize,
        event: &MouseDownEvent,
        cx: &mut Context<'_, Self>,
    ) {
        if event.button != MouseButton::Left {
            return;
        }
        self.press = Some(Press {
            index,
            origin: event.position,
        });
        cx.notify();
    }

    pub(crate) fn moved(
        &mut self,
        event: &MouseMoveEvent,
        _: &mut Window,
        cx: &mut Context<'_, Self>,
    ) {
        if event.pressed_button != Some(MouseButton::Left) {
            self.cancel_press(cx);
            return;
        }
        let (now, motion) = (Instant::now(), Motion::of(cx));
        let x = self.content_x(event.position);
        if let Some(drag) = &mut self.drag {
            drag.engine.moved(x);
            drag.follow(event.position, now, motion);
            cx.notify();
            return;
        }
        let Some(press) = self
            .press
            .filter(|press| press.has_moved_past_threshold(event.position))
        else {
            return;
        };
        self.begin_drag(press, event.position, cx);
    }

    pub(crate) fn released(
        &mut self,
        _: &MouseUpEvent,
        _: &mut Window,
        cx: &mut Context<'_, Self>,
    ) {
        let press = self.press.take();
        self.incoming = None;
        let Some(drag) = self.drag.take() else {
            self.activate(press, cx);
            return;
        };
        self.settle_into_slot(&drag, cx);
        let drop = drag.engine.drop();
        cx.notify();
        if let Some(command) = drop {
            cx.emit(TabBarEvent::Apply(vec![command]));
        }
    }

    fn activate(&mut self, press: Option<Press>, cx: &mut Context<'_, Self>) {
        let Some(tab) = press.and_then(|press| self.tabs.get(press.index)) else {
            return;
        };
        let command = Command::ActivateTab { tab: tab.id };
        cx.emit(TabBarEvent::Apply(vec![command]));
    }

    pub(crate) fn content_x(&self, position: Point<Pixels>) -> f32 {
        let scrolled = self.scroll.offset().x;
        f32::from(position.x - self.strip_left() - scrolled)
    }

    fn cancel_press(&mut self, cx: &mut Context<'_, Self>) {
        if self.press.take().is_some() || self.drag.take().is_some() {
            cx.notify();
        }
    }

    fn begin_drag(&mut self, press: Press, position: Point<Pixels>, cx: &mut Context<'_, Self>) {
        let ids = self.tabs.iter().map(|tab| tab.id).collect();
        let spans = self.slot_spans();
        let x = self.content_x(press.origin);
        let Some(engine) = StripDrag::begin(ids, spans, press.index, x) else {
            return;
        };
        let grab_y =
            f32::from(press.origin.y - self.row.origin.y) - strip_metrics::SCROLLER_PADDING;
        self.drag = Some(DragSession::new(engine, position, grab_y, self.tabs.len()));
        cx.notify();
    }

    fn settle_into_slot(&mut self, drag: &DragSession, cx: &Context<'_, Self>) {
        let Some(tab) = drag.engine.tab() else {
            return;
        };
        let x = self.content_x(drag.pointer);
        let slot = self
            .slot_spans()
            .get(drag.engine.target())
            .map_or(0.0, |span| span.x);
        let from = drag.engine.ghost_x(x) - slot;
        self.settle = Some(Settle::new(tab, from, Instant::now(), Motion::of(cx)));
    }
}

#[cfg(test)]
#[path = "tab_bar_pointer_tests.rs"]
mod tests;
