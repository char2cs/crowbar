//! The boxed click callback shared by interactive components.

use gpui::{App, ClickEvent, Window};

/// Callback invoked on a mouse or keyboard activation.
pub(crate) type ClickHandler = Box<dyn Fn(&ClickEvent, &mut Window, &mut App) + 'static>;
