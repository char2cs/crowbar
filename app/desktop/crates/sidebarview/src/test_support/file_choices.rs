//! A file chooser that remembers each question and answers with one path.

use std::cell::RefCell;
use std::path::PathBuf;
use std::rc::Rc;

use gpui::{Task, TestAppContext};
use ui::{ChooseFiles, FileChoice, FileChooser};

pub(crate) type Asked = Rc<RefCell<Vec<FileChoice>>>;

pub(crate) fn answer_choices_with(
    cx: &mut TestAppContext,
    path: &str,
) -> Asked {
    let asked = Asked::default();
    let heard = Rc::clone(&asked);
    let path = PathBuf::from(path);
    let choose: ChooseFiles = Rc::new(move |choice, _, _| {
        heard.borrow_mut().push(choice);
        Task::ready(vec![path.clone()])
    });
    cx.update(|cx| cx.set_global(FileChooser::new(choose)));
    asked
}
