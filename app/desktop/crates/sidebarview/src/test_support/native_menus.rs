//! A native menu that remembers each request and answers with one entry.

use std::cell::RefCell;
use std::rc::Rc;

use gpui::{Task, TestAppContext};
use ui::{MenuPath, NativeEntry, NativeMenu, NativeRequest, PopupMenu};

pub(crate) type Popped = Rc<RefCell<Vec<NativeRequest>>>;

pub(crate) fn answer_menus_with(
    cx: &mut TestAppContext,
    path: Option<MenuPath>,
) -> Popped {
    let popped = Popped::default();
    let heard = Rc::clone(&popped);
    let popup: PopupMenu = Rc::new(move |request, _, _| {
        heard.borrow_mut().push(request);
        Task::ready(path.clone())
    });
    cx.update(|cx| cx.set_global(NativeMenu::new(popup)));
    popped
}

pub(crate) fn labels(entries: &[NativeEntry]) -> Vec<String> {
    entries
        .iter()
        .map(|entry| match entry {
            NativeEntry::Item { label, .. } | NativeEntry::Submenu { label, .. } => {
                label.to_string()
            },
            NativeEntry::Separator => "-".to_owned(),
        })
        .collect()
}
