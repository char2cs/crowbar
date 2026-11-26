//! A row drag, of chats or of Files rows, ends without a drop when the pointer comes back with no button held (released
//! outside the window) or the window loses focus, as develop's `use-sidebar-drag.ts` does.

use gpui::{Context, MouseMoveEvent, Window};

use crate::sidebar::Sidebar;

impl Sidebar {
    pub(crate) fn track_pointer(
        &mut self,
        event: &MouseMoveEvent,
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) {
        self.move_resize(event, cx);
        self.track_drag_buttons(event, window, cx);
        self.track_peek(event.position, window, cx);
    }

    fn track_drag_buttons(
        &mut self,
        event: &MouseMoveEvent,
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) {
        let pressed = self.drag.press.is_some() || self.file_drag.is_live();
        if event.pressed_button.is_none() && pressed {
            self.cancel_drag(window, cx);
        }
    }

    /// Ends a row drag without a drop: the window lost focus mid-drag.
    pub fn cancel_drag(
        &mut self,
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) {
        cx.stop_active_drag(window);
        self.end_drag(cx);
        self.end_file_drag(cx);
    }
}

#[cfg(test)]
mod tests {
    use gpui::{Bounds, point, px, size};
    use sidebar::RowId;

    use crate::sidebar::{DragPoint, DragPress};
    use crate::test_support::prelude::*;

    fn lifted(
        sidebar: &gpui::Entity<crate::sidebar::Sidebar>,
        cx: &mut gpui::VisualTestContext,
    ) {
        let press = DragPress {
            row: RowId::from("working"),
            at: point(px(0.0), px(200.0)),
        };
        let over = DragPoint {
            subject: RowId::from("working"),
            target: RowId::from("folder"),
            bounds: Bounds::new(point(px(0.0), px(200.0)), size(px(200.0), px(36.0))),
            at: point(px(20.0), px(218.0)),
        };
        sidebar.update(cx, |this, cx| {
            this.press_row(press);
            this.drag_over(&over, cx);
        });
    }

    fn still_lifted(
        sidebar: &gpui::Entity<crate::sidebar::Sidebar>,
        cx: &mut gpui::VisualTestContext,
    ) -> bool {
        sidebar.read_with(cx, |this, _| this.drag.lifted.is_some())
    }

    #[gpui::test]
    fn a_move_with_no_button_held_ends_the_drag_and_a_held_one_keeps_it(cx: &mut TestAppContext) {
        let (recorder, sidebar, cx) = open(cx, DemoShell::rich());
        lifted(&sidebar, cx);
        let moved = |held| gpui::MouseMoveEvent {
            position: point(px(20.0), px(30.0)),
            pressed_button: held,
            modifiers: gpui::Modifiers::default(),
        };
        sidebar.update_in(cx, |this, window, cx| {
            this.track_drag_buttons(&moved(Some(gpui::MouseButton::Left)), window, cx);
        });
        assert!(still_lifted(&sidebar, cx), "held");
        sidebar.update_in(cx, |this, window, cx| {
            this.track_drag_buttons(&moved(None), window, cx);
        });
        assert!(!still_lifted(&sidebar, cx), "released outside");
        assert_eq!(recorder.sent(), vec![], "nothing dropped");
    }

    #[gpui::test]
    fn losing_window_focus_ends_the_drag(cx: &mut TestAppContext) {
        let (recorder, sidebar, cx) = open(cx, DemoShell::rich());
        lifted(&sidebar, cx);
        sidebar.update_in(cx, crate::sidebar::Sidebar::cancel_drag);
        assert!(!still_lifted(&sidebar, cx));
        assert_eq!(recorder.sent(), vec![]);
    }
}
