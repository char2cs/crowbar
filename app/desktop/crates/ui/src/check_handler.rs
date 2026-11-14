//! The boxed callback of a two-state control.

use gpui::{App, Window};

/// Callback receiving the new on/off value.
pub(crate) type ToggleHandler = Box<dyn Fn(bool, &mut Window, &mut App) + 'static>;
