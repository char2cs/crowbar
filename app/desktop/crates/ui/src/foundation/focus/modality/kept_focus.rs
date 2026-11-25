//! A tab-stop focus handle an element keeps across frames, so its focus ring can be decided
//! from it.

use gpui::{App, ElementId, FocusHandle, Window};

pub(crate) fn kept_focus(
    id: &ElementId,
    window: &mut Window,
    cx: &mut App,
) -> FocusHandle {
    let kept = window.use_keyed_state((id.clone(), "focus"), cx, |_, cx| {
        cx.focus_handle().tab_stop(true)
    });
    kept.read(cx).clone()
}

pub(crate) fn or_kept(
    given: Option<FocusHandle>,
    id: &ElementId,
    window: &mut Window,
    cx: &mut App,
) -> FocusHandle {
    given.unwrap_or_else(|| kept_focus(id, window, cx))
}

#[cfg(test)]
mod tests {
    use std::cell::RefCell;
    use std::rc::Rc;

    use gpui::{InteractiveElement as _, Styled as _, TestAppContext, div, px};

    use super::*;
    use crate::test_support::probe::open;

    #[gpui::test]
    fn the_kept_handle_is_a_tab_stop_and_the_same_on_every_frame(cx: &mut TestAppContext) {
        let seen: Rc<RefCell<Vec<FocusHandle>>> = Rc::default();
        let sink = Rc::clone(&seen);
        let cx = open(cx, move |window, cx| {
            let focus = kept_focus(&ElementId::from("kept"), window, cx);
            sink.borrow_mut().push(focus.clone());
            div().size(px(10.0)).track_focus(&focus)
        });
        cx.update(|window, _| window.refresh());
        cx.update(Window::focus_next);
        cx.run_until_parked();
        let handles = seen.borrow().clone();
        assert!(handles.len() > 1, "drawn more than once");
        assert!(
            handles.windows(2).all(|pair| pair[0] == pair[1]),
            "a new handle on a later frame"
        );
        assert!(
            cx.update(|window, _| handles[0].is_focused(window)),
            "not a tab stop"
        );
    }
}
