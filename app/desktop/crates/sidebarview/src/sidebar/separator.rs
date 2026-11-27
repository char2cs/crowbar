use gpui::{Context, KeyDownEvent, Modifiers, MouseMoveEvent, Pixels, Window};
use sidebar::{KeyStep, Panel, Presentation, ResizeDrag, ResizeKey, Resizing, Visibility};

use crate::sidebar::Sidebar;

fn step_of(modifiers: Modifiers) -> KeyStep {
    if modifiers.shift {
        return KeyStep::Coarse;
    }
    KeyStep::Fine
}

impl Sidebar {
    pub(crate) fn container_width(window: &Window) -> f32 {
        f32::from(window.viewport_size().width)
    }

    pub(crate) fn live_width(
        &self,
        container: f32,
    ) -> f32 {
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
        let drag = ResizeDrag::begin(width, f32::from(press_x), self.panel, container);
        self.resizing = Some(Resizing { drag, width });
        window.focus(&self.handle_focus, cx);
        cx.notify();
    }

    pub(crate) fn move_resize(
        &mut self,
        event: &MouseMoveEvent,
        cx: &mut Context<'_, Self>,
    ) {
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

    pub(crate) fn end_resize(
        &mut self,
        cx: &mut Context<'_, Self>,
    ) {
        let Some(resizing) = self.resizing.take() else {
            return;
        };
        self.panel.commit(resizing.width);
        self.changed(cx);
    }

    pub(crate) fn toggle_collapsed(
        &mut self,
        cx: &mut Context<'_, Self>,
    ) {
        self.panel.toggle();
        self.changed(cx);
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
        let step = step_of(event.keystroke.modifiers);
        let container = Self::container_width(window);
        let current = self.live_width(container);
        let width = key.apply(step, self.panel, (current, container));
        self.panel.commit(width);
        if self.panel.visibility() == Visibility::Closed {
            self.peek.slide_to(0.0, ui::Motion::of(cx));
        }
        self.changed(cx);
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::test_support::prelude::*;

    #[gpui::test]
    fn dragging_changes_the_live_width_and_releasing_stores_it(cx: &mut TestAppContext) {
        let (_, sidebar, cx) = open(cx, DemoShell::reference());
        sidebar.update_in(cx, |this, window, cx| {
            this.begin_resize(px(294.0), window, cx);
        });
        let event = MouseMoveEvent {
            position: point(px(400.0), px(300.0)),
            pressed_button: Some(gpui::MouseButton::Left),
            modifiers: Modifiers::none(),
        };
        sidebar.update(cx, |this, cx| this.move_resize(&event, cx));
        assert_eq!(
            sidebar.read_with(cx, |this, _| this.live_width(1200.0)),
            400.0
        );
        sidebar.update(cx, Sidebar::end_resize);
        assert_eq!(
            sidebar.read_with(cx, |this, _| this.panel.preferred()),
            400.0
        );
    }

    #[gpui::test]
    fn dragging_below_half_the_minimum_collapses(cx: &mut TestAppContext) {
        let (_, sidebar, cx) = open(cx, DemoShell::reference());
        sidebar.update_in(cx, |this, window, cx| {
            this.begin_resize(px(294.0), window, cx);
        });
        let event = MouseMoveEvent {
            position: point(px(50.0), px(300.0)),
            pressed_button: Some(gpui::MouseButton::Left),
            modifiers: Modifiers::none(),
        };
        sidebar.update(cx, |this, cx| this.move_resize(&event, cx));
        sidebar.update(cx, Sidebar::end_resize);
        assert_eq!(
            sidebar.read_with(cx, |this, _| this.panel.visibility()),
            Visibility::Closed
        );
    }

    #[gpui::test]
    fn the_home_key_collapses_and_end_widens(cx: &mut TestAppContext) {
        let (_, sidebar, cx) = open(cx, DemoShell::reference());
        let press = |key: &'static str| KeyDownEvent {
            keystroke: gpui::Keystroke::parse(key).expect("a key"),
            is_held: false,
            prefer_character_input: false,
        };
        sidebar.update_in(cx, |this, window, cx| {
            this.press_separator(&press("end"), window, cx);
        });
        assert!(sidebar.read_with(cx, |this, _| this.panel.preferred()) > 294.0);
        sidebar.update_in(cx, |this, window, cx| {
            this.press_separator(&press("home"), window, cx);
        });
        assert_eq!(
            sidebar.read_with(cx, |this, _| this.panel.visibility()),
            Visibility::Closed
        );
    }

    #[gpui::test]
    fn toggling_flips_between_open_and_closed(cx: &mut TestAppContext) {
        let (_, sidebar, cx) = open(cx, DemoShell::reference());
        sidebar.update(cx, Sidebar::toggle_collapsed);
        assert_eq!(
            sidebar.read_with(cx, |this, _| this.panel.visibility()),
            Visibility::Closed
        );
    }
}
