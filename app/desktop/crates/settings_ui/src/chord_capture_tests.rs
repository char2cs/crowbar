use super::*;
use crate::modifier::Modifier;

fn press(key: &str, held: &[Modifier]) -> Pressed {
    Pressed::new(key, held)
}

#[test]
fn idle_capture_ignores_everything() {
    let mut capture = ChordCapture::default();
    assert_eq!(
        capture.feed(&press("t", &[Modifier::Control]), Primary::Control),
        None
    );
    assert_eq!(capture.target(), None);
}

#[test]
fn an_accepted_chord_ends_capture_and_names_the_command() {
    let mut capture = ChordCapture::default();
    capture.begin("tabs.new");
    let done = capture.feed(&press("n", &[Modifier::Control]), Primary::Control);
    assert_eq!(
        done.map(|(command, chord)| (command, chord.as_str().to_owned())),
        Some(("tabs.new".to_owned(), "mod+n".to_owned()))
    );
    assert_eq!(capture.target(), None);
}

#[test]
fn refusals_keep_capturing_and_remember_why() {
    let mut capture = ChordCapture::default();
    capture.begin("tabs.new");
    assert_eq!(capture.feed(&press("a", &[]), Primary::Control), None);
    assert_eq!(capture.target(), Some("tabs.new"));
    assert_eq!(capture.refusal(), Some(Refusal::BareKey));
    assert_eq!(
        capture.feed(&press("control", &[Modifier::Control]), Primary::Control),
        None
    );
    assert_eq!(
        capture.target(),
        Some("tabs.new"),
        "a bare modifier keeps waiting"
    );
}

#[test]
fn escape_and_a_second_begin_both_leave_capture() {
    let mut capture = ChordCapture::default();
    capture.begin("tabs.new");
    assert_eq!(capture.feed(&press("escape", &[]), Primary::Control), None);
    assert_eq!(capture.target(), None);
    capture.begin("tabs.new");
    capture.begin("tabs.new");
    assert_eq!(
        capture.target(),
        None,
        "pressing the capturing button again toggles it off"
    );
    capture.begin("tabs.new");
    capture.begin("tabs.close");
    assert_eq!(
        capture.target(),
        Some("tabs.close"),
        "another button takes over"
    );
}
