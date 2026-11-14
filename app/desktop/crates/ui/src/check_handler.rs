//! The boxed callback of a two-state control.

use gpui::{App, Window};

pub(crate) type ToggleHandler = Box<dyn Fn(bool, &mut Window, &mut App) + 'static>;
