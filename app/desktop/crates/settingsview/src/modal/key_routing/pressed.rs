//! Translates GPUI key presses into the capture rule's vocabulary.

use gpui::Keystroke;

use settings::{ModifierKey, Pressed};

pub(crate) fn pressed(keystroke: &Keystroke) -> Pressed {
    let flags = [
        (keystroke.modifiers.control, ModifierKey::Control),
        (keystroke.modifiers.alt, ModifierKey::Alt),
        (keystroke.modifiers.shift, ModifierKey::Shift),
        (keystroke.modifiers.platform, ModifierKey::Platform),
    ];
    let held: Vec<ModifierKey> = flags
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
    use chord::Chord;
    use chord::Primary;
    use settings::Capture;

    fn captured(source: &'static str) -> Capture {
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

}
