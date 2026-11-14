use super::*;
use crate::capture::Capture;
use crate::chord::Chord;
use crate::primary::Primary;

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
