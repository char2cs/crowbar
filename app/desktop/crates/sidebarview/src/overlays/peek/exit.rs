use gpui::{
    AnyElement, App, Bounds, Context, Entity, IntoElement, MouseExitEvent, Pixels, Point, Styled,
    Window, canvas,
};

use crate::sidebar::Sidebar;

fn listen(
    sidebar: &Entity<Sidebar>,
    window: &mut Window,
) {
    let sidebar = sidebar.clone();
    window.on_mouse_event(move |event: &MouseExitEvent, _, window, cx: &mut App| {
        let at = event.position;
        sidebar.update(cx, |this, cx| this.pointer_left_window(at, window, cx));
    });
}

impl Sidebar {
    fn pointer_left_window(
        &mut self,
        at: Point<Pixels>,
        window: &Window,
        cx: &mut Context<'_, Self>,
    ) {
        let inside = Bounds::new(Point::default(), window.viewport_size()).contains(&at);
        let (x, y) = (f32::from(at.x), f32::from(at.y));
        if inside && self.peek_zone(window).keeps(x, y) {
            return;
        }
        self.pointer_left_peek(cx);
    }

    pub(crate) fn exit_listener(cx: &Context<'_, Self>) -> AnyElement {
        let sidebar = cx.entity();
        canvas(
            |_, _, _| {},
            move |_, (), window, _| listen(&sidebar, window),
        )
        .absolute()
        .size_0()
        .into_any_element()
    }
}

#[cfg(test)]
mod tests {
    use gpui::{Modifiers, MouseExitEvent, point, px};

    use super::*;
    use sidebar::DemoShell;

    use crate::test_support::open;

    #[gpui::test]
    fn the_pointer_leaving_the_window_dismisses_the_peek_card(cx: &mut gpui::TestAppContext) {
        let (_, sidebar, cx) = open(cx, DemoShell::reference());
        sidebar.update_in(cx, Sidebar::toggle);
        sidebar.update(cx, Sidebar::begin_peek);
        cx.simulate_event(MouseExitEvent {
            position: point(px(-1.0), px(40.0)),
            pressed_button: None,
            modifiers: Modifiers::none(),
        });
        assert!(!sidebar.read_with(cx, |this, _| this.peek.is_wanted()));
    }

    #[gpui::test]
    fn the_pointer_crossing_onto_the_card_keeps_it(cx: &mut gpui::TestAppContext) {
        let mut floating = crate::test_support::open_floating(cx, DemoShell::reference());
        let (sidebar, cx) = (floating.sidebar.clone(), &mut floating.window);
        sidebar.update_in(cx, Sidebar::toggle);
        sidebar.update(cx, Sidebar::begin_peek);
        cx.simulate_event(MouseExitEvent {
            position: point(px(100.0), px(300.0)),
            pressed_button: None,
            modifiers: Modifiers::none(),
        });
        assert!(
            sidebar.read_with(cx, |this, _| this.peek.is_wanted()),
            "the pointer went onto a web view inside the kept area"
        );
    }
}
