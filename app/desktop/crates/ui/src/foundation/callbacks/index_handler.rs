//! The shared callback receiving an item index.

use std::rc::Rc;

use gpui::{App, Window};

pub(crate) type IndexHandler = Rc<dyn Fn(usize, &mut Window, &mut App) + 'static>;
