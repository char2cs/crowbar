use gpui::TestAppContext;
use ui::testing::{Popped, answer_menus};

pub(crate) fn answer_menus_with(
    cx: &mut TestAppContext,
    path: Option<ui::MenuPath>,
) -> Popped {
    answer_menus(cx, move |_| path.clone())
}
