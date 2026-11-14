use super::*;

fn outcome(key: &str, held: &[Modifier]) -> Capture {
    Capture::interpret(&Pressed::new(key, held), Primary::Control)
}

fn accepted(text: &str) -> Capture {
    Capture::Accepted(Chord::parse(text).unwrap())
}

#[test]
fn accepted_chords() {
    assert_eq!(outcome("t", &[Modifier::Control]), accepted("mod+t"));
    assert_eq!(
        outcome("t", &[Modifier::Control, Modifier::Shift]),
        accepted("mod+shift+t")
    );
    assert_eq!(outcome("n", &[Modifier::Alt]), accepted("alt+n"));
    assert_eq!(
        outcome("arrowleft", &[Modifier::Control, Modifier::Alt]),
        accepted("mod+alt+arrowleft")
    );
    assert_eq!(outcome("f5", &[]), accepted("f5"));
    assert_eq!(outcome("f12", &[Modifier::Shift]), accepted("shift+f12"));
    assert_eq!(
        outcome("escape", &[Modifier::Control]),
        accepted("mod+escape")
    );
}

#[test]
fn bare_printable_and_navigation_keys_are_refused() {
    for key in [
        "a",
        "1",
        "`",
        "space",
        "enter",
        "tab",
        "backspace",
        "arrowup",
        "f25",
        "f",
    ] {
        assert_eq!(
            outcome(key, &[]),
            Capture::Refused(Refusal::BareKey),
            "{key}"
        );
    }
    assert_eq!(
        outcome("a", &[Modifier::Shift]),
        Capture::Refused(Refusal::BareKey),
        "shift alone is typing"
    );
}

#[test]
fn escape_cancels_and_modifier_keys_wait() {
    assert_eq!(outcome("escape", &[]), Capture::Cancelled);
    for key in ["control", "shift", "alt", "meta", "platform"] {
        assert_eq!(
            outcome(key, &[Modifier::Control]),
            Capture::Ignored,
            "{key}"
        );
    }
}

#[test]
fn the_other_primary_modifier_is_not_in_the_grammar() {
    assert_eq!(
        outcome("t", &[Modifier::Platform]),
        Capture::Refused(Refusal::UnsupportedModifier)
    );
    let mac = Capture::interpret(&Pressed::new("t", &[Modifier::Platform]), Primary::Platform);
    assert_eq!(mac, accepted("mod+t"));
}
