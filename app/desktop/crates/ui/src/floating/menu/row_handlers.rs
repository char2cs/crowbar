//! The callbacks a menu row reports to.

use std::rc::Rc;

use gpui::{App, Window};

pub(crate) type RowHandler = Rc<dyn Fn(usize, &mut Window, &mut App)>;

#[derive(Clone)]
pub(crate) struct RowHandlers {
    pub(crate) hover: RowHandler,
    pub(crate) choose: RowHandler,
}
