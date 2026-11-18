//! Translates GPUI key presses into the capture rule's vocabulary.

use gpui::Keystroke;

use crate::modifier::Modifier;
use crate::pressed::Pressed;

pub(crate) fn pressed(keystroke: &Keystroke) -> Pressed {
    let flags = [
        (keystroke.modifiers.control, Modifier::Control),
        (keystroke.modifiers.alt, Modifier::Alt),
        (keystroke.modifiers.shift, Modifier::Shift),
        (keystroke.modifiers.platform, Modifier::Platform),
    ];
    let held: Vec<Modifier> = flags
        .iter()
        .filter(|(on, _)| *on)
        .map(|&(_, modifier)| modifier)
        .collect();
    Pressed::new(dom_name(&keystroke.key), &held)
}

fn dom_name(key: &str) -> &str {
    match key {
        "left" => "arrowleft",
        "right" => "arrowright",
        "up" => "arrowup",
        "down" => "arrowdown",
        other => other,
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::capture::Capture;
    use chord::Chord;
    use chord::Primary;

    fn captured(source: &str) -> Capture {
        let keystroke = Keystroke::parse(source).unwrap();
        Capture::interpret(&pressed(&keystroke), Primary::Control)
    }

    fn chord(text: &str) -> Capture {
        Capture::Accepted(Chord::parse(text).unwrap())
    }

    #[test]
    fn gpui_keystrokes_become_chords() {
        const CASES: &[(&str, &str)] = &[
            ("ctrl-t", "mod+t"),
            ("ctrl-shift-t", "mod+shift+t"),
            ("ctrl-alt-left", "mod+alt+arrowleft"),
            ("alt-n", "alt+n"),
            ("f5", "f5"),
            ("ctrl-`", "mod+`"),
            ("ctrl-=", "mod+="),
        ];
        for &(keystroke, expected) in CASES {
            assert_eq!(captured(keystroke), chord(expected), "{keystroke}");
        }
    }

    #[test]
    fn bare_keys_are_still_refused_after_translation() {
        assert!(matches!(captured("a"), Capture::Refused(_)));
        assert!(matches!(captured("left"), Capture::Refused(_)));
        assert_eq!(captured("escape"), Capture::Cancelled);
    }
}
