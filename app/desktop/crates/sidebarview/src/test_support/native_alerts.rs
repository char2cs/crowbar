use std::cell::RefCell;
use std::rc::Rc;

use gpui::{Task, TestAppContext};
use ui::{AlertRequest, AskAlert, NativeAlert};

pub(crate) type Asked = Rc<RefCell<Vec<AlertRequest>>>;

pub(crate) fn answer_alerts_with(
    cx: &mut TestAppContext,
    pressed: Option<usize>,
) -> Asked {
    let asked = Asked::default();
    let heard = Rc::clone(&asked);
    let ask: AskAlert = Rc::new(move |request, _, _| {
        heard.borrow_mut().push(request);
        Task::ready(pressed)
    });
    cx.update(|cx| cx.set_global(NativeAlert::new(ask)));
    asked
}
