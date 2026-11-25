use gpui::{App, Window};

use crate::layout_view::pane::handle_button::HandleAction;

pub(crate) fn route(
    actions: [&HandleAction; 2],
    hovered: usize,
    window: &mut Window,
    cx: &mut App,
) {
    if let Some(action) = actions.get(hovered) {
        action(window, cx);
    }
}

#[cfg(test)]
mod tests {
    use std::cell::RefCell;
    use std::rc::Rc;

    use gpui::TestAppContext;

    use super::*;

    fn recording(
        name: &'static str,
        seen: &Rc<RefCell<Vec<&'static str>>>,
    ) -> HandleAction {
        let seen = Rc::clone(seen);
        Rc::new(move |_, _| seen.borrow_mut().push(name))
    }

    #[gpui::test]
    fn a_hover_change_runs_the_leave_action_then_the_enter_action_by_its_state(
        cx: &mut TestAppContext
    ) {
        let seen = Rc::new(RefCell::new(Vec::new()));
        let (leave, enter) = (recording("leave", &seen), recording("enter", &seen));
        let window = cx.add_empty_window();
        window.update(|window, cx| {
            route([&leave, &enter], usize::from(true), window, cx);
            route([&leave, &enter], usize::from(false), window, cx);
        });
        assert_eq!(*seen.borrow(), ["enter", "leave"], "by hover state");
    }
}
