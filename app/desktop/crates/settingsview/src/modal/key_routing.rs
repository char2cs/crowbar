//! Which key presses the modal itself handles.

mod mode;
mod pressed;

pub(crate) use mode::Mode;

use gpui::Keystroke;
use settings::RailMove;

use crate::intent::Intent;
use crate::modal::key_routing::pressed::pressed;

pub(crate) fn route(
    keystroke: &Keystroke,
    mode: Mode,
) -> Option<Intent> {
    if mode == Mode::Capturing {
        return Some(Intent::Key(pressed(keystroke)));
    }
    if keystroke.key == "escape" {
        return Some(Intent::Close);
    }
    if mode != Mode::Rail {
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
        mode: Mode,
    ) -> Option<Intent> {
        route(&Keystroke::parse(keystroke).unwrap(), mode)
    }

    #[test]
    fn capturing_takes_every_key_including_escape() {
        assert!(matches!(
            routed("escape", Mode::Capturing),
            Some(Intent::Key(_))
        ));
        assert!(matches!(routed("a", Mode::Capturing), Some(Intent::Key(_))));
    }

    #[test]
    fn escape_closes_from_the_rail_and_from_the_page() {
        assert!(matches!(routed("escape", Mode::Rail), Some(Intent::Close)));
        assert!(matches!(routed("escape", Mode::Page), Some(Intent::Close)));
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
                matches!(routed(key, Mode::Rail), Some(Intent::Move(m)) if m == expected),
                "{key}"
            );
            assert!(routed(key, Mode::Page).is_none(), "{key}");
        }
        assert!(routed("a", Mode::Rail).is_none());
    }
}
