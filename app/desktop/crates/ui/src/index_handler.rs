//! The shared callback receiving an item index.

use std::rc::Rc;

use gpui::{App, Window};

/// Callback invoked with the index of the chosen item.
pub(crate) type IndexHandler = Rc<dyn Fn(usize, &mut Window, &mut App) + 'static>;
