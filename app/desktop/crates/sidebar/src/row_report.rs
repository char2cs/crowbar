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
#[path = "row_report_tests.rs"]
mod tests;
