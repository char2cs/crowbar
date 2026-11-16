//! The boxed callback of a two-state control.

use gpui::{App, Window};

use crate::pose::Pose;

pub(crate) type ToggleHandler = Box<dyn Fn(Pose, &mut Window, &mut App) + 'static>;
