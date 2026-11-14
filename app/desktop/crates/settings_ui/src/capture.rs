//! The rule for turning a key press into a chord.

use crate::chord::Chord;
use crate::modifier::Modifier;
use crate::pressed::Pressed;
use crate::primary::Primary;
use crate::refusal::Refusal;

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
    pub fn interpret(pressed: &Pressed, primary: Primary) -> Self {
        if MODIFIER_KEYS.contains(&pressed.key.as_str()) {
            return Self::Ignored;
        }
        if pressed.held.is_empty() && pressed.key == "escape" {
            return Self::Cancelled;
        }
        if let Some(refusal) = unsupported(pressed, primary) {
            return Self::Refused(refusal);
        }
        let chorded = pressed.has(primary_modifier(primary)) || pressed.has(Modifier::Alt);
        if !chorded && !is_function_key(&pressed.key) {
            return Self::Refused(Refusal::BareKey);
        }
        match Chord::parse(&spell(pressed, primary)) {
            Some(chord) => Self::Accepted(chord),
            None => Self::Refused(Refusal::BareKey),
        }
    }
}

fn primary_modifier(primary: Primary) -> Modifier {
    match primary {
        Primary::Control => Modifier::Control,
        Primary::Platform => Modifier::Platform,
    }
}

fn unsupported(pressed: &Pressed, primary: Primary) -> Option<Refusal> {
    let other = match primary {
        Primary::Control => Modifier::Platform,
        Primary::Platform => Modifier::Control,
    };
    pressed.has(other).then_some(Refusal::UnsupportedModifier)
}

fn is_function_key(key: &str) -> bool {
    key.strip_prefix('f')
        .and_then(|digits| digits.parse::<u8>().ok())
        .is_some_and(|number| (1..=LAST_FUNCTION_KEY).contains(&number))
}

fn spell(pressed: &Pressed, primary: Primary) -> String {
    let words = [
        (primary_modifier(primary), "mod"),
        (Modifier::Shift, "shift"),
        (Modifier::Alt, "alt"),
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
#[path = "capture_tests.rs"]
mod tests;
