//! A mouse report on its way to the program.

use gpui::MouseButton as HostButton;
use terminal::{MouseButton, ViewPoint};

use crate::terminal_view::input::key_conversion::terminal_modifiers;

#[derive(Debug, Clone, Copy)]
pub(crate) struct Report {
    pub(crate) button: MouseButton,
    pub(crate) point: ViewPoint,
    pub(crate) modifiers: terminal::Modifiers,
}

pub(crate) fn reported_button(button: HostButton) -> Option<MouseButton> {
    match button {
        HostButton::Left => Some(MouseButton::Left),
        HostButton::Middle => Some(MouseButton::Middle),
        HostButton::Right => Some(MouseButton::Right),
        HostButton::Navigate(_) => None,
    }
}

pub(crate) fn pointer_report(
    button: MouseButton,
    point: ViewPoint,
    modifiers: gpui::Modifiers,
) -> Report {
    Report {
        button,
        point,
        modifiers: terminal_modifiers(modifiers),
    }
}
#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn host_buttons_map_to_terminal_buttons_and_navigation_buttons_are_not_reported() {
        let cases = [
            (HostButton::Left, Some(MouseButton::Left)),
            (HostButton::Middle, Some(MouseButton::Middle)),
            (HostButton::Right, Some(MouseButton::Right)),
            (HostButton::Navigate(gpui::NavigationDirection::Back), None),
        ];
        for (host, expected) in cases {
            assert_eq!(reported_button(host), expected, "{host:?}");
        }
    }
}
