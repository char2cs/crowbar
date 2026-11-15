//! A normalised key chord such as `mod+shift+t`.

use crate::primary::Primary;

const MODIFIER_COUNT: usize = 3;
const MODIFIERS: [&str; MODIFIER_COUNT] = ["mod", "shift", "alt"];

/// A chord in the canonical grammar: lowercase, `+`-separated, order `mod`, `shift`, `alt`, key.
#[derive(Debug, Clone, PartialEq, Eq, Hash)]
pub struct Chord(String);

impl Chord {
    /// Parses and normalises `text`; `None` when it has no key or repeats a modifier.
    pub fn parse(text: &str) -> Option<Self> {
        let lowered = text.trim().to_lowercase();
        let mut rest = lowered.as_str();
        let mut present = [false; MODIFIER_COUNT];
        while let Some((slot, tail)) = take_modifier(rest) {
            claim(&mut present, slot)?;
            rest = tail;
        }
        let dangling = rest.len() > 1 && rest.ends_with('+');
        let keyless = rest.is_empty() || MODIFIERS.contains(&rest);
        if keyless || dangling {
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
    pub fn label(
        &self,
        primary: Primary,
    ) -> String {
        let parts: Vec<String> = self.0.split('+').map(|part| word(part, primary)).collect();
        match primary {
            Primary::Control => parts.join("+"),
            Primary::Platform => parts.concat(),
        }
    }
}

fn claim(
    present: &mut [bool; MODIFIER_COUNT],
    slot: usize,
) -> Option<()> {
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

fn word(
    part: &str,
    primary: Primary,
) -> String {
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
mod tests {
    use super::*;

    #[test]
    fn parse_normalises_order_and_case() {
        const CASES: &[(&str, Option<&str>)] = &[
            ("Mod+Shift+T", Some("mod+shift+t")),
            ("alt+mod+n", Some("mod+alt+n")),
            ("shift+alt+mod+arrowLeft", Some("mod+shift+alt+arrowleft")),
            ("mod+`", Some("mod+`")),
            ("mod++", Some("mod++")),
            ("f5", Some("f5")),
            ("mod+mod+t", None),
            ("mod+", None),
            ("", None),
            ("shift", None),
        ];
        for &(text, expected) in CASES {
            assert_eq!(
                Chord::parse(text).as_ref().map(Chord::as_str),
                expected,
                "{text:?}"
            );
        }
    }

    #[test]
    fn labels_follow_the_platform() {
        let chord = Chord::parse("mod+shift+alt+arrowleft").unwrap();
        assert_eq!(chord.label(Primary::Control), "Ctrl+Shift+Alt+\u{2190}");
        assert_eq!(
            chord.label(Primary::Platform),
            "\u{2318}\u{21e7}\u{2325}\u{2190}"
        );
        assert_eq!(
            Chord::parse("mod+t").unwrap().label(Primary::Control),
            "Ctrl+T"
        );
    }
}
