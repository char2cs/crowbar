//! Where the keyboard is inside an open dialog.

use gpui::{App, FocusHandle, Window};

#[derive(Clone)]
pub(crate) struct Focus {
    pub(crate) root: FocusHandle,
    pub(crate) close: FocusHandle,
}

impl Focus {
    pub(crate) fn opening(
        window: &mut Window,
        cx: &mut App,
    ) -> Self {
        let focus = Self {
            root: cx.focus_handle(),
            close: cx.focus_handle().tab_stop(true),
        };
        let first = focus.clone();
        window.defer(cx, move |window, cx| first.focus_first(window, cx));
        focus
    }

    fn focus_first(
        &self,
        window: &mut Window,
        cx: &App,
    ) {
        if self.root.contains_focused(window, cx) {
            return;
        }
        if self.root.contains(&self.close, window) {
            return window.focus(&self.close);
        }
        window.focus(&self.root);
    }

    pub(crate) fn step(
        &self,
        step: fn(&mut Window),
        window: &mut Window,
        cx: &App,
    ) {
        step(window);
        if self.root.contains_focused(window, cx) {
            return;
        }
        window.focus(&self.close);
    }
}

#[cfg(test)]
mod tests {
    use std::cell::RefCell;
    use std::rc::Rc;

    use gpui::{InteractiveElement, ParentElement, Styled, TestAppContext, div, px};

    use super::*;
    use crate::test_support::probe::open;

    fn stop(focus: &FocusHandle) -> gpui::Div {
        div()
            .size(px(5.0))
            .track_focus(&focus.clone().tab_stop(true))
    }

    #[gpui::test]
    fn an_opening_dialog_focuses_its_close_button_and_tab_wraps_inside(cx: &mut TestAppContext) {
        let held: Rc<RefCell<Option<Focus>>> = Rc::default();
        let (other, outside) = cx.update(|cx| (cx.focus_handle(), cx.focus_handle()));
        let (keep, inner, after) = (Rc::clone(&held), other.clone(), outside.clone());
        let window = open(cx, move |window, cx| {
            let focus = keep
                .borrow_mut()
                .get_or_insert_with(|| Focus::opening(window, cx))
                .clone();
            let dialog = div()
                .track_focus(&focus.root)
                .child(stop(&focus.close))
                .child(stop(&inner));
            div().child(dialog).child(stop(&after))
        });
        window.run_until_parked();
        let focus = held
            .borrow()
            .clone()
            .unwrap_or_else(|| panic!("the dialog opened"));
        assert!(window.update(|window, _| focus.close.is_focused(window)));
        window.update(|window, cx| {
            window.focus(&other);
            focus.step(Window::focus_next, window, cx);
        });
        assert!(window.update(|window, _| focus.close.is_focused(window)));
    }
}
