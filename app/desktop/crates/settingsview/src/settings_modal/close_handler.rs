//! The callback that closes the modal.

use std::rc::Rc;

use gpui::{App, Window};

pub(crate) type CloseHandler = Rc<dyn Fn(&mut Window, &mut App)>;
