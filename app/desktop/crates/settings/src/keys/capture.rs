//! The rule for turning a key press into a chord.

use crate::keys::modifier_key::ModifierKey;
use crate::keys::pressed::Pressed;
use crate::keys::refusal::Refusal;
use chord::Chord;
use chord::Primary;

const MODIFIER_KEYS: [&str; 6] = ["control", "alt", "shift", "meta", "platform", "function"];
const LAST_FUNCTION_KEY: u8 = 24;

/// What a key press does while a shortcut is being captured.
#[derive(Debug, Clone, PartialEq, Eq)]
#[non_exhaustive]
pub enum Capture {
    /// A modifier on its own: keep waiting.
    Ignored,
    /// Escape without a modifier: leave capture, change nothing.
    Cancelled,
    /// Not allowed: keep waiting and tell the user why.
    Refused(Refusal),
    /// The chord to store.
    Accepted(Chord),
}

impl Capture {
    /// Applies the rule: a chord needs `mod` or Alt, or is a function key; bare Escape cancels; bare printable keys are refused.
    pub fn interpret(
        pressed: &Pressed,
        primary: Primary,
    ) -> Self {
        if MODIFIER_KEYS.contains(&pressed.key.as_str()) {
            return Self::Ignored;
        }
        if pressed.held.is_empty() && pressed.key == "escape" {
            return Self::Cancelled;
        }
        if let Some(refusal) = unsupported(pressed, primary) {
            return Self::Refused(refusal);
        }
        let chorded = pressed.has(ModifierKey::primary(primary)) || pressed.has(ModifierKey::Alt);
        if !chorded && !is_function_key(&pressed.key) {
            return Self::Refused(Refusal::BareKey);
        }
        match Chord::parse(&spell(pressed, primary)) {
            Some(chord) => Self::Accepted(chord),
            None => Self::Refused(Refusal::BareKey),
        }
    }
}

fn unsupported(
    pressed: &Pressed,
    primary: Primary,
) -> Option<Refusal> {
    let other = match primary {
        Primary::Control => ModifierKey::Platform,
        Primary::Platform => ModifierKey::Control,
    };
    pressed.has(other).then_some(Refusal::UnsupportedModifier)
}

fn is_function_key(key: &str) -> bool {
    key.strip_prefix('f')
        .and_then(|digits| digits.parse::<u8>().ok())
        .is_some_and(|number| (1..=LAST_FUNCTION_KEY).contains(&number))
}

fn spell(
    pressed: &Pressed,
    primary: Primary,
) -> String {
    let words = [
        (ModifierKey::primary(primary), "mod"),
        (ModifierKey::Shift, "shift"),
        (ModifierKey::Alt, "alt"),
    ];
    let mut parts: Vec<&str> = words
        .iter()
        .filter(|(modifier, _)| pressed.has(*modifier))
        .map(|&(_, word)| word)
        .collect();
    parts.push(&pressed.key);
    parts.join("+")
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::test_support::{Case, run_cases};

    fn outcome(
        key: &str,
        held: &[ModifierKey],
    ) -> Capture {
        Capture::interpret(&Pressed::new(key, held), Primary::Control)
    }

    fn accepted(text: &str) -> Capture {
        Capture::Accepted(Chord::parse(text).unwrap())
    }

    fn accepted_chords() {
        assert_eq!(outcome("t", &[ModifierKey::Control]), accepted("mod+t"));
        assert_eq!(
            outcome("t", &[ModifierKey::Control, ModifierKey::Shift]),
            accepted("mod+shift+t")
        );
        assert_eq!(outcome("n", &[ModifierKey::Alt]), accepted("alt+n"));
        assert_eq!(
            outcome("arrowleft", &[ModifierKey::Control, ModifierKey::Alt]),
            accepted("mod+alt+arrowleft")
        );
        assert_eq!(outcome("f5", &[]), accepted("f5"));
        assert_eq!(outcome("f12", &[ModifierKey::Shift]), accepted("shift+f12"));
        assert_eq!(
            outcome("escape", &[ModifierKey::Control]),
            accepted("mod+escape")
        );
    }

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
            outcome("a", &[ModifierKey::Shift]),
            Capture::Refused(Refusal::BareKey),
            "shift alone is typing"
        );
    }

    fn escape_cancels_and_modifier_keys_wait() {
        assert_eq!(outcome("escape", &[]), Capture::Cancelled);
        for key in ["control", "shift", "alt", "meta", "platform"] {
            assert_eq!(
                outcome(key, &[ModifierKey::Control]),
                Capture::Ignored,
                "{key}"
            );
        }
    }

    fn the_other_primary_modifier_is_not_in_the_grammar() {
        assert_eq!(
            outcome("t", &[ModifierKey::Platform]),
            Capture::Refused(Refusal::UnsupportedModifier)
        );
        let mac = Capture::interpret(
            &Pressed::new("t", &[ModifierKey::Platform]),
            Primary::Platform,
        );
        assert_eq!(mac, accepted("mod+t"));
    }

    const CASES: &[Case] = &[
        ("accepted chords", accepted_chords),
        (
            "bare printable and navigation keys are refused",
            bare_printable_and_navigation_keys_are_refused,
        ),
        (
            "escape cancels and modifier keys wait",
            escape_cancels_and_modifier_keys_wait,
        ),
        (
            "the other primary modifier is not in the grammar",
            the_other_primary_modifier_is_not_in_the_grammar,
        ),
    ];

    #[test]
    fn every_scenario_holds() {
        run_cases(CASES);
    }
}
