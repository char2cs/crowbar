use std::cell::RefCell;
use std::path::PathBuf;
use std::rc::Rc;

use gpui::{Task, TestAppContext};

pub(crate) type Asked = Rc<RefCell<Vec<ui::FileChoice>>>;

pub(crate) fn answer_choices_with(
    cx: &mut TestAppContext,
    paths: &[&str],
) -> Asked {
    let asked = Asked::default();
    let heard = Rc::clone(&asked);
    let paths: Vec<PathBuf> = paths.iter().map(PathBuf::from).collect();
    let choose: ui::ChooseFiles = Rc::new(move |choice, _, _| {
        heard.borrow_mut().push(choice);
        Task::ready(paths.clone())
    });
    cx.update(|cx| cx.set_global(ui::FileChooser::new(choose)));
    asked
}
