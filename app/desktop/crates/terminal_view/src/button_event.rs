//! A mouse button together with the event it was pressed or released with.

use gpui::MouseButton;

#[derive(Debug)]
pub(crate) struct ButtonEvent<'a, Event> {
    pub(crate) button: MouseButton,
    pub(crate) event: &'a Event,
}

impl<Event> Clone for ButtonEvent<'_, Event> {
    fn clone(&self) -> Self {
        *self
    }
}

impl<Event> Copy for ButtonEvent<'_, Event> {}
