//! Converts GPUI keystrokes to the terminal crate's key input.

use gpui::Keystroke;
use terminal::{KeyInput, Modifiers};

pub(crate) fn key_input(keystroke: &Keystroke) -> KeyInput {
    let modifiers = modifiers_of(keystroke.modifiers);
    let text = match modifiers.contains(Modifiers::ALT) {
        true => meta_text(keystroke, modifiers),
        false => keystroke.key_char.clone(),
    };
    KeyInput {
        key: keystroke.key.clone(),
        text,
        modifiers,
    }
}

fn meta_text(
    keystroke: &Keystroke,
    modifiers: Modifiers,
) -> Option<String> {
    let mut characters = keystroke.key.chars();
    let (Some(only), None) = (characters.next(), characters.next()) else {
        return keystroke.key_char.clone();
    };
    if modifiers.contains(Modifiers::SHIFT) {
        return Some(only.to_uppercase().collect());
    }
    Some(only.to_string())
}

fn modifiers_of(held: gpui::Modifiers) -> Modifiers {
    let flags = [
        (held.shift, Modifiers::SHIFT),
        (held.alt, Modifiers::ALT),
        (held.control, Modifiers::CONTROL),
        (held.platform, Modifiers::PLATFORM),
    ];
    flags
        .into_iter()
        .filter(|(down, _)| *down)
        .fold(Modifiers::NONE, |all, (_, modifier)| all.with(modifier))
}

pub(crate) fn terminal_modifiers(held: gpui::Modifiers) -> Modifiers {
    modifiers_of(held)
}

#[cfg(test)]
mod tests {
    use super::*;

    fn keystroke(source: &str) -> Keystroke {
        Keystroke::parse(source).unwrap_or_else(|_| Keystroke {
            modifiers: gpui::Modifiers::default(),
            key: source.to_owned(),
            key_char: None,
        })
    }

    #[test]
    fn modifiers_and_text_carry_over() {
        let plain = key_input(&keystroke("a"));
        assert_eq!(
            (plain.key.as_str(), plain.modifiers),
            ("a", Modifiers::NONE)
        );
        let control = key_input(&keystroke("ctrl-c"));
        assert!(control.modifiers.contains(Modifiers::CONTROL));
    }

    #[test]
    fn alt_types_the_key_not_the_composed_character() {
        let mut stroke = keystroke("alt-s");
        stroke.key_char = Some("ß".to_owned());
        assert_eq!(key_input(&stroke).text.as_deref(), Some("s"));
        let mut shifted = keystroke("alt-shift-s");
        shifted.key_char = Some("Í".to_owned());
        assert_eq!(key_input(&shifted).text.as_deref(), Some("S"));
    }
}
