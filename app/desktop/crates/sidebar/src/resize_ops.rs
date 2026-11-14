//! Pointer and keyboard control of the separator.

use gpui::{Context, KeyDownEvent, MouseMoveEvent, Pixels, Window};

use crate::key_step::KeyStep;
use crate::panel::Panel;
use crate::presentation::Presentation;
use crate::resize_drag::ResizeDrag;
use crate::resize_key::ResizeKey;
use crate::resizing::Resizing;
use crate::sidebar::Sidebar;
use crate::visibility::Visibility;

impl Sidebar {
    pub(crate) fn container_width(window: &Window) -> f32 {
        f32::from(window.viewport_size().width)
    }

    pub(crate) fn live_width(&self, container: f32) -> f32 {
        self.resizing.map_or_else(
            || self.panel.track_width(container),
            |resizing| resizing.width,
        )
    }

    pub(crate) fn begin_resize(
        &mut self,
        press_x: Pixels,
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) {
        let container = Self::container_width(window);
        if Panel::presentation(container) == Presentation::Overlay {
            return;
        }
        let width = self.live_width(container);
        let drag = ResizeDrag::begin(width, f32::from(press_x), &self.panel, container);
        self.resizing = Some(Resizing { drag, width });
        window.focus(&self.handle_focus);
        cx.notify();
    }

    pub(crate) fn move_resize(&mut self, event: &MouseMoveEvent, cx: &mut Context<'_, Self>) {
        let Some(resizing) = self.resizing.as_mut() else {
            return;
        };
        if event.pressed_button.is_none() {
            self.end_resize(cx);
            return;
        }
        resizing.width = resizing.drag.width_at(f32::from(event.position.x));
        cx.notify();
    }

    pub(crate) fn end_resize(&mut self, cx: &mut Context<'_, Self>) {
        let Some(resizing) = self.resizing.take() else {
            return;
        };
        self.panel.commit(resizing.width);
        Self::changed(cx);
    }

    pub(crate) fn toggle_collapsed(&mut self, cx: &mut Context<'_, Self>) {
        self.panel.toggle();
        Self::changed(cx);
    }

    pub(crate) fn press_separator(
        &mut self,
        event: &KeyDownEvent,
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) {
        let key = match event.keystroke.key.as_str() {
            "left" => ResizeKey::Left,
            "right" => ResizeKey::Right,
            "home" => ResizeKey::Home,
            "end" => ResizeKey::End,
            "enter" => ResizeKey::Enter,
            _ => return,
        };
        let step = if event.keystroke.modifiers.shift {
            KeyStep::Coarse
        } else {
            KeyStep::Fine
        };
        let container = Self::container_width(window);
        let current = self.live_width(container);
        let width = key.apply(step, self.panel, (current, container));
        self.panel.commit(width);
        if self.panel.visibility() == Visibility::Closed {
            self.peek.slide_to(0.0, ui::Motion::of(cx));
        }
        Self::changed(cx);
    }
}

#[cfg(test)]
#[path = "resize_ops_tests.rs"]
mod tests;
