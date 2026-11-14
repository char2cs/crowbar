//! Which key presses the modal itself handles.

use gpui::Keystroke;

use crate::intent::Intent;
use crate::key_mode::KeyMode;
use crate::keys::pressed;
use crate::rail_move::RailMove;

pub(crate) fn route(keystroke: &Keystroke, mode: KeyMode) -> Option<Intent> {
    if mode == KeyMode::Capturing {
        return Some(Intent::Key(pressed(keystroke)));
    }
    if keystroke.key == "escape" {
        return Some(Intent::Close);
    }
    if mode != KeyMode::Rail {
        return None;
    }
    let movement = match keystroke.key.as_str() {
        "down" | "right" => RailMove::Next,
        "up" | "left" => RailMove::Previous,
        "home" => RailMove::First,
        "end" => RailMove::Last,
        _ => return None,
    };
    Some(Intent::Move(movement))
}

#[cfg(test)]
#[path = "key_routing_tests.rs"]
mod tests;
