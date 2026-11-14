//! A normalised key chord such as `mod+shift+t`.

use crate::primary::Primary;

const MODIFIERS: [&str; 3] = ["mod", "shift", "alt"];

/// A chord in the canonical grammar: lowercase, `+`-separated, order `mod`, `shift`, `alt`, key.
#[derive(Debug, Clone, PartialEq, Eq, Hash)]
pub struct Chord(String);

impl Chord {
    /// Parses and normalises `text`; `None` when it has no key or repeats a modifier.
    pub fn parse(text: &str) -> Option<Self> {
        let lowered = text.trim().to_lowercase();
        let mut rest = lowered.as_str();
        let mut present = [false; 3];
        while let Some((slot, tail)) = take_modifier(rest) {
            claim(&mut present, slot)?;
            rest = tail;
        }
        let dangling = rest.len() > 1 && rest.ends_with('+');
        if rest.is_empty() || dangling || MODIFIERS.contains(&rest) {
            return None;
        }
        let words = MODIFIERS
            .iter()
            .zip(present)
            .filter(|&(_, on)| on)
            .map(|(word, _)| *word);
        Some(Self(words.chain([rest]).collect::<Vec<_>>().join("+")))
    }

    /// The canonical text, which is also what is stored.
    pub fn as_str(&self) -> &str {
        &self.0
    }

    /// The chord as shown to the user: `Ctrl+Shift+T`, or `⇧⌘T` where `mod` is Command.
    pub fn label(&self, primary: Primary) -> String {
        let parts: Vec<String> = self.0.split('+').map(|part| word(part, primary)).collect();
        match primary {
            Primary::Control => parts.join("+"),
            Primary::Platform => parts.concat(),
        }
    }
}

fn claim(present: &mut [bool; 3], slot: usize) -> Option<()> {
    let flag = present.get_mut(slot)?;
    let repeated = std::mem::replace(flag, true);
    (!repeated).then_some(())
}

fn take_modifier(text: &str) -> Option<(usize, &str)> {
    MODIFIERS.iter().enumerate().find_map(|(slot, word)| {
        let tail = text.strip_prefix(word)?.strip_prefix('+')?;
        (!tail.is_empty()).then_some((slot, tail))
    })
}

fn word(part: &str, primary: Primary) -> String {
    match (part, primary) {
        ("mod", Primary::Control) => "Ctrl".to_owned(),
        ("mod", Primary::Platform) => "\u{2318}".to_owned(),
        ("shift", Primary::Control) => "Shift".to_owned(),
        ("shift", Primary::Platform) => "\u{21e7}".to_owned(),
        ("alt", Primary::Control) => "Alt".to_owned(),
        ("alt", Primary::Platform) => "\u{2325}".to_owned(),
        (key, _) => key_label(key),
    }
}

fn key_label(key: &str) -> String {
    match key {
        "arrowleft" => "\u{2190}".to_owned(),
        "arrowright" => "\u{2192}".to_owned(),
        "arrowup" => "\u{2191}".to_owned(),
        "arrowdown" => "\u{2193}".to_owned(),
        other => other.to_uppercase(),
    }
}

#[cfg(test)]
#[path = "chord_tests.rs"]
mod tests;
