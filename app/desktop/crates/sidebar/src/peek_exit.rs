//! Dismisses the peek card when the pointer leaves the window.

use gpui::{AnyElement, App, Context, Entity, IntoElement, MouseExitEvent, Styled, Window, canvas};

use crate::sidebar::Sidebar;

fn listen(
    sidebar: &Entity<Sidebar>,
    window: &mut Window,
) {
    let sidebar = sidebar.clone();
    window.on_mouse_event(move |_: &MouseExitEvent, _, _, cx: &mut App| {
        sidebar.update(cx, Sidebar::pointer_left_window);
    });
}

impl Sidebar {
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
    use crate::demo_data::DemoData;
    use crate::test_support::open;

    #[gpui::test]
    fn the_pointer_leaving_the_window_dismisses_the_peek_card(cx: &mut gpui::TestAppContext) {
        let (_, sidebar, cx) = open(cx, DemoData::reference());
        sidebar.update(cx, Sidebar::toggle);
        sidebar.update(cx, Sidebar::begin_peek);
        cx.simulate_event(MouseExitEvent {
            position: point(px(-1.0), px(40.0)),
            pressed_button: None,
            modifiers: Modifiers::none(),
        });
        assert!(!sidebar.read_with(cx, |this, _| this.peek.is_wanted()));
    }
}
