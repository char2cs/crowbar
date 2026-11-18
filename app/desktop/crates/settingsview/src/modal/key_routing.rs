//! Which key presses the modal itself handles.

mod key_mode;
mod pressed;

pub(crate) use key_mode::KeyMode;

use gpui::Keystroke;
use settings::RailMove;

use crate::intent::Intent;
use crate::modal::key_routing::pressed::pressed;

pub(crate) fn route(
    keystroke: &Keystroke,
    mode: KeyMode,
) -> Option<Intent> {
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
mod tests {
    use super::*;

    fn routed(
        keystroke: &str,
        mode: KeyMode,
    ) -> Option<Intent> {
        route(&Keystroke::parse(keystroke).unwrap(), mode)
    }

    #[test]
    fn capturing_takes_every_key_including_escape() {
        assert!(matches!(
            routed("escape", KeyMode::Capturing),
            Some(Intent::Key(_))
        ));
        assert!(matches!(
            routed("a", KeyMode::Capturing),
            Some(Intent::Key(_))
        ));
    }

    #[test]
    fn escape_closes_from_the_rail_and_from_the_page() {
        assert!(matches!(
            routed("escape", KeyMode::Rail),
            Some(Intent::Close)
        ));
        assert!(matches!(
            routed("escape", KeyMode::Page),
            Some(Intent::Close)
        ));
    }

    #[test]
    fn arrows_move_the_rail_only_while_it_has_focus() {
        const CASES: &[(&str, RailMove)] = &[
            ("down", RailMove::Next),
            ("right", RailMove::Next),
            ("up", RailMove::Previous),
            ("left", RailMove::Previous),
            ("home", RailMove::First),
            ("end", RailMove::Last),
        ];
        for &(key, expected) in CASES {
            assert!(
                matches!(routed(key, KeyMode::Rail), Some(Intent::Move(m)) if m == expected),
                "{key}"
            );
            assert!(routed(key, KeyMode::Page).is_none(), "{key}");
        }
        assert!(routed("a", KeyMode::Rail).is_none());
    }
}
