//! Click listeners that report a fixed action to the sidebar.

use gpui::{App, ClickEvent, Window};

use crate::sidebar::{RowAction, RowHandler};

pub(crate) fn report(
    handler: &RowHandler,
    action: RowAction,
) -> impl Fn(&ClickEvent, &mut Window, &mut App) + 'static {
    let handler = std::rc::Rc::clone(handler);
    move |_, window, cx| {
        cx.stop_propagation();
        handler(action.clone(), window, cx);
    }
}

#[cfg(test)]
mod tests {
    use gpui::{InteractiveElement, ParentElement, StatefulInteractiveElement, Styled, div, px};

    use super::*;
    use crate::test_support::{mount, recording_handler};
    use sidebar::RowId;

    #[gpui::test]
    fn a_click_reports_its_action_to_the_handler(cx: &mut gpui::TestAppContext) {
        let (handler, seen) = recording_handler();
        let cx = mount(cx, move |_, _| {
            let click = report(&handler, RowAction::Open(RowId::from("chat")));
            div().id("button").size(px(40.0)).on_click(click).child("x")
        });
        cx.simulate_click(gpui::point(px(10.0), px(10.0)), gpui::Modifiers::none());
        assert_eq!(*seen.borrow(), vec![RowAction::Open(RowId::from("chat"))]);
    }
}
