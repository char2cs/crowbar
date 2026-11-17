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

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn a_copy_names_the_same_button_and_event() {
        let event = 7_u8;
        let pressed = ButtonEvent {
            button: MouseButton::Left,
            event: &event,
        };
        let copy = pressed;
        assert_eq!(copy.button, pressed.button);
        assert!(std::ptr::eq(copy.event, pressed.event));
    }
}
