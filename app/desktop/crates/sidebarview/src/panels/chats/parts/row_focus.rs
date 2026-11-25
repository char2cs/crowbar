//! Keyboard focus of a tree row: a tab stop that rings on focus-visible and ignores pointer presses.

use gpui::prelude::FluentBuilder;
use gpui::{App, Div, ElementId, FocusHandle, InteractiveElement, Stateful, Styled, Window};
use ui::{Palette, Pose, focus_ring};

pub(crate) fn row_focus(
    id: &ElementId,
    window: &mut Window,
    cx: &mut App,
) -> FocusHandle {
    let state = window.use_keyed_state((id.clone(), "focus"), cx, |_, cx| {
        cx.focus_handle().tab_stop(true)
    });
    state.read(cx).clone()
}

pub(crate) fn ringed(
    shell: Stateful<Div>,
    focus: &FocusHandle,
    palette: &Palette,
    rung: Pose,
) -> Stateful<Div> {
    let ring = focus_ring(palette);
    shell
        .track_focus(focus)
        .when(rung == Pose::On, move |style| style.shadow(ring))
        .on_mouse_down(gpui::MouseButton::Left, |_, window, _| {
            window.prevent_default();
        })
}

#[cfg(test)]
mod tests {
    use std::cell::RefCell;
    use std::rc::Rc;

    use gpui::{ParentElement, TestAppContext};
    use ui::{Surface, Theme};

    use super::*;
    use crate::panels::chats::parts::row_shell::row_shell;
    use crate::test_support::mount;

    #[gpui::test]
    fn a_row_is_a_tab_stop_that_a_click_does_not_focus(cx: &mut TestAppContext) {
        let seen: Rc<RefCell<Vec<FocusHandle>>> = Rc::default();
        let sink = Rc::clone(&seen);
        let cx = mount(cx, move |window, cx| {
            let palette = Theme::of(cx).palette(Surface::Chrome);
            let id = ElementId::from("row");
            let focus = row_focus(&id, window, cx);
            sink.borrow_mut().push(focus.clone());
            let shell = row_shell(id, 0, palette).child("label");
            let rung = Pose::from(ui::Modality::rings(&focus, window, cx));
            ringed(shell, &focus, palette, rung)
        });
        cx.simulate_click(
            gpui::point(gpui::px(20.0), gpui::px(10.0)),
            gpui::Modifiers::none(),
        );
        cx.run_until_parked();
        let handles = seen.borrow().clone();
        let clicked = cx.update(|window, _| handles.iter().any(|handle| handle.is_focused(window)));
        cx.update(Window::focus_next);
        cx.run_until_parked();
        let focused = cx.update(|window, _| handles.iter().any(|handle| handle.is_focused(window)));
        let distinct = handles.windows(2).all(|pair| pair[0] == pair[1]);
        assert!(!clicked, "a pointer press focused the row");
        assert!(focused, "the row is not a tab stop");
        assert!(distinct, "the row got a new handle on a later frame");
    }

    fn rings_drawn(cx: &mut gpui::VisualTestContext) -> usize {
        cx.run_until_parked();
        cx.update(|window, cx| {
            let ring = Theme::of(cx).palette(Surface::Chrome).ring();
            let shadows = &window.rendered_scene().shadows;
            shadows.iter().filter(|shadow| shadow.color == ring).count()
        })
    }

    #[gpui::test]
    fn a_focused_row_rings_only_after_a_key_as_develop_s_focus_visible_does(
        cx: &mut TestAppContext
    ) {
        let focus = cx.update(|cx| cx.focus_handle());
        let held = focus.clone();
        let cx = mount(cx, move |window, cx| {
            let palette = Theme::of(cx).palette(Surface::Chrome);
            let rung = Pose::from(ui::Modality::rings(&held, window, cx));
            ringed(
                row_shell("row".into(), 0, palette).child("label"),
                &held,
                palette,
                rung,
            )
        });
        cx.simulate_click(
            gpui::point(gpui::px(20.0), gpui::px(10.0)),
            gpui::Modifiers::none(),
        );
        cx.update(|window, cx| window.focus(&focus, cx));
        assert_eq!(rings_drawn(cx), 0, "focused after a pointer press");
        cx.simulate_keystrokes("shift");
        assert_eq!(rings_drawn(cx), 1, "focused after a key");
    }
}
