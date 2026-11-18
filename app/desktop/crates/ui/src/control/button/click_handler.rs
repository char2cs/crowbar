//! The boxed click callback shared by interactive components.

use gpui::{App, ClickEvent, Window};

pub(crate) type ClickHandler = Box<dyn Fn(&ClickEvent, &mut Window, &mut App) + 'static>;
