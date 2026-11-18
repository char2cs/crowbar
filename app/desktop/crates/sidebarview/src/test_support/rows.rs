//! Row values and handlers the row tests share.

use sidebar::{Row, RowKind};
use std::cell::RefCell;
use std::rc::Rc;

use crate::sidebar::{RowAction, RowHandler};

pub(crate) type Seen = Rc<RefCell<Vec<RowAction>>>;

pub(crate) fn chat(label: &str) -> Row {
    Row::new("chat", RowKind::Chat, 1, label)
}

pub(crate) fn recording_handler() -> (RowHandler, Seen) {
    let seen = Seen::default();
    let sink = Rc::clone(&seen);
    (
        Rc::new(move |action, _, _| sink.borrow_mut().push(action)),
        seen,
    )
}
