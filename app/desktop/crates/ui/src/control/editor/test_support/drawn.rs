use std::cell::Cell;
use std::rc::Rc;

use gpui::{Entity, VisualTestContext};

pub(crate) type Drawn<'cx, T> = (Entity<T>, Rc<Cell<usize>>, &'cx mut VisualTestContext);
