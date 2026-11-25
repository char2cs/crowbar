use std::cell::RefCell;
use std::rc::Rc;

use gpui::{Task, TestAppContext};

pub(crate) type Popped = Rc<RefCell<Vec<ui::NativeRequest>>>;

pub(crate) fn answer_menus_with(
    cx: &mut TestAppContext,
    path: Option<ui::MenuPath>,
) -> Popped {
    let popped = Popped::default();
    let heard = Rc::clone(&popped);
    let popup: ui::PopupMenu = Rc::new(move |request, _, _| {
        heard.borrow_mut().push(request);
        Task::ready(path.clone())
    });
    cx.update(|cx| cx.set_global(ui::NativeMenu::new(popup)));
    popped
}
