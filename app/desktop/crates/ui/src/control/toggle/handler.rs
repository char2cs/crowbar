//! The boxed callback of a two-state control.

use gpui::{App, Window};

use crate::foundation::Pose;

pub(crate) type Handler = Box<dyn Fn(Pose, &mut Window, &mut App) + 'static>;
