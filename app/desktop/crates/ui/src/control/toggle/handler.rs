use gpui::{App, Window};

use crate::foundation::Pose;

pub(crate) type Handler = Box<dyn Fn(Pose, &mut Window, &mut App) + 'static>;
