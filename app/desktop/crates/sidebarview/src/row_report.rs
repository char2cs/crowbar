//! Click listeners that report a fixed action to the sidebar.

use gpui::{App, ClickEvent, Window};

use crate::row_action::{RowAction, RowHandler};

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
    use std::cell::RefCell;
    use std::rc::Rc;

    use gpui::{
        Context, InteractiveElement, IntoElement, ParentElement, Render,
        StatefulInteractiveElement, Styled, TestAppContext, Window, div, px,
    };

    use super::*;
    use sidebar::RowId;

    struct Button {
        handler: RowHandler,
    }

    impl Render for Button {
        fn render(
            &mut self,
            _: &mut Window,
            _: &mut Context<'_, Self>,
        ) -> impl IntoElement {
            let click = report(&self.handler, RowAction::Open(RowId::from("chat")));
            div().id("button").size(px(40.0)).on_click(click).child("x")
        }
    }

    #[gpui::test]
    fn a_click_reports_its_action_to_the_handler(cx: &mut TestAppContext) {
        let seen: Rc<RefCell<Vec<RowAction>>> = Rc::default();
        let sink = Rc::clone(&seen);
        let handler: RowHandler = Rc::new(move |action, _, _| sink.borrow_mut().push(action));
        let (_, cx) = cx.add_window_view(|_, _| Button { handler });
        cx.simulate_click(gpui::point(px(10.0), px(10.0)), gpui::Modifiers::none());
        assert_eq!(*seen.borrow(), vec![RowAction::Open(RowId::from("chat"))]);
    }
}
