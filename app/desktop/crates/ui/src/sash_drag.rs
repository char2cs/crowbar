//! Drag payload of a [`crate::Sash`].

use gpui::{Context, IntoElement, Render, Window, div};

use crate::axis::Axis;

/// Payload attached while a sash is dragged; the owner listens with `on_drag_move::<SashDrag>`.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub struct SashDrag {
    pub(crate) axis: Axis,
}

impl SashDrag {
    /// The axis of the sash being dragged.
    pub fn axis(&self) -> Axis {
        self.axis
    }
}

impl Render for SashDrag {
    fn render(&mut self, _window: &mut Window, _cx: &mut Context<Self>) -> impl IntoElement {
        div()
    }
}
