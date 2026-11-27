//! A native menu that records what it is asked for and answers a fixed choice.

use gpui::TestAppContext;

use crate::floating::MenuPath;
use crate::testing::{Popped, answer_menus};

pub(crate) fn answering(
    cx: &mut TestAppContext,
    path: Option<MenuPath>,
) -> Popped {
    answer_menus(cx, move |_| path.clone())
}
